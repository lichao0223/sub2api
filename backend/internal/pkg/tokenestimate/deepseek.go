package tokenestimate

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/dlclark/regexp2/v2"
)

//go:embed deepseek/tokenizer.json
var deepSeekTokenizerJSON []byte

type deepSeekTokenizer struct {
	merges map[tokenPair]tokenMerge
	splits []*regexp2.Regexp
	added  *regexp.Regexp
	byteID [256]int
}

var deepSeek = sync.OnceValue(func() *deepSeekTokenizer {
	t, err := newDeepSeekTokenizer()
	if err != nil {
		panic(fmt.Errorf("load embedded DeepSeek tokenizer: %w", err))
	}
	return t
})

func newDeepSeekTokenizer() (*deepSeekTokenizer, error) {
	var config struct {
		Model struct {
			Vocab  map[string]int `json:"vocab"`
			Merges []string       `json:"merges"`
		} `json:"model"`
		AddedTokens []struct {
			Content string `json:"content"`
		} `json:"added_tokens"`
		PreTokenizer struct {
			PreTokenizers []struct {
				Type    string `json:"type"`
				Pattern struct {
					Regex string `json:"Regex"`
				} `json:"pattern"`
			} `json:"pretokenizers"`
		} `json:"pre_tokenizer"`
	}
	if err := json.Unmarshal(deepSeekTokenizerJSON, &config); err != nil {
		return nil, err
	}
	t := &deepSeekTokenizer{merges: make(map[tokenPair]tokenMerge, len(config.Model.Merges))}
	for rank, merge := range config.Model.Merges {
		left, right, ok := strings.Cut(merge, " ")
		leftID, leftOK := config.Model.Vocab[left]
		rightID, rightOK := config.Model.Vocab[right]
		id, idOK := config.Model.Vocab[left+right]
		if !ok || !leftOK || !rightOK || !idOK {
			return nil, fmt.Errorf("invalid DeepSeek merge %d", rank)
		}
		t.merges[tokenPair{leftID, rightID}] = tokenMerge{rank: rank, id: id}
	}
	for _, split := range config.PreTokenizer.PreTokenizers {
		if split.Type != "Split" {
			continue // ByteLevel is applied after all isolated regex splits.
		}
		re, err := regexp2.Compile(split.Pattern.Regex, regexp2.None)
		if err != nil {
			return nil, err
		}
		t.splits = append(t.splits, re)
	}
	added := make([]string, 0, len(config.AddedTokens))
	for _, token := range config.AddedTokens {
		added = append(added, regexp.QuoteMeta(token.Content))
	}
	re, err := regexp.Compile(strings.Join(added, "|"))
	if err != nil {
		return nil, err
	}
	t.added = re
	t.added.Longest()
	// Hugging Face ByteLevel maps non-printing bytes to code points above 255.
	n := rune(256)
	for b := range t.byteID {
		r := rune(b)
		if b < 33 || b >= 127 && b <= 160 || b == 173 {
			r = n
			n++
		}
		id, ok := config.Model.Vocab[string(r)]
		if !ok {
			return nil, fmt.Errorf("missing DeepSeek byte %d", b)
		}
		t.byteID[b] = id
	}
	return t, nil
}

func (t *deepSeekTokenizer) count(text string, limit int) (int, error) {
	total := 0
	var scratch bpeScratch
	var splitRunes func([]rune, int) error
	splitRunes = func(runes []rune, stage int) error {
		if len(runes) == 0 || total > limit {
			return nil
		}
		if stage == len(t.splits) {
			total += t.countPiece(string(runes), &scratch)
			return nil
		}
		re := t.splits[stage]
		match, err := re.FindRunesMatch(runes)
		start := 0
		for err == nil && match != nil && total <= limit {
			if err = splitRunes(runes[start:match.RuneIndex], stage+1); err != nil {
				return err
			}
			end := match.RuneIndex + match.RuneLength
			if err = splitRunes(runes[match.RuneIndex:end], stage+1); err != nil {
				return err
			}
			start = end
			match, err = re.FindNextMatch(match)
		}
		if err != nil {
			return err
		}
		return splitRunes(runes[start:], stage+1)
	}
	var split func(string, int) error
	split = func(text string, stage int) error {
		if text == "" || total > limit {
			return nil
		}
		if stage == len(t.splits) {
			total += t.countPiece(text, &scratch)
			return nil
		}
		runes := []rune(text)
		re := t.splits[stage]
		match, err := re.FindRunesMatch(runes)
		start := 0
		for err == nil && match != nil && total <= limit {
			if err = split(string(runes[start:match.RuneIndex]), stage+1); err != nil {
				return err
			}
			if err = split(match.String(), stage+1); err != nil {
				return err
			}
			start = match.RuneIndex + match.RuneLength
			match, err = re.FindNextMatch(match)
		}
		if err != nil {
			return err
		}
		return split(string(runes[start:]), stage+1)
	}
	start := 0
	addedMatches := t.added.FindAllStringIndex(text, -1)
	if len(addedMatches) == 0 {
		if err := splitRunes([]rune(text), 0); err != nil {
			return 0, err
		}
		return min(total, limit+1), nil
	}
	for _, match := range addedMatches {
		if err := split(text[start:match[0]], 0); err != nil {
			return 0, err
		}
		total++
		if total > limit {
			return limit + 1, nil
		}
		start = match[1]
	}
	if err := split(text[start:], 0); err != nil {
		return 0, err
	}
	return min(total, limit+1), nil
}
