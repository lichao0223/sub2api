package tokenestimate

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeQueuePush(t *testing.T) {
	var queue mergeQueue
	require.Panics(t, func() { queue.Push(nil) })
	want := mergeCandidate{left: 1, right: 2}
	queue.Push(want)
	require.Equal(t, want, queue[0])
}

func TestDeepSeekTokenizerGolden(t *testing.T) {
	raw, err := os.ReadFile("deepseek/golden.json")
	require.NoError(t, err)
	var samples []struct {
		Text   string `json:"text"`
		Tokens int    `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(raw, &samples))
	for i, sample := range samples {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			n, err := deepSeek().count(sample.Text, sample.Tokens)
			require.NoError(t, err)
			require.Equal(t, sample.Tokens, n)
			if sample.Tokens > 1 {
				n, err = deepSeek().count(sample.Text, sample.Tokens-2)
				require.NoError(t, err)
				require.Equal(t, sample.Tokens-1, n, "must stop at limit+1")
			}
		})
	}
}

func TestDeepSeekTokenizerAddedTokens(t *testing.T) {
	var config struct {
		AddedTokens []struct {
			Content string `json:"content"`
		} `json:"added_tokens"`
	}
	require.NoError(t, json.Unmarshal(deepSeekTokenizerJSON, &config))
	for _, token := range config.AddedTokens {
		n, err := deepSeek().count(token.Content, 100)
		require.NoError(t, err)
		require.Equal(t, 1, n, token.Content)
	}
}
