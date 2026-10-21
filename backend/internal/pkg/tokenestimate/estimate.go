// Package tokenestimate provides a cheap input estimate for admission.
package tokenestimate

import (
	"math"
	"strings"
	"unicode"

	"github.com/tidwall/gjson"
)

// Count stops at limit+1. Its result is a lower bound when the limit is exceeded.
// ASCII characters count as roughly 0.3 token and Han characters as roughly 0.6 token,
// following DeepSeek public estimation guidance. Other Unicode uses one token.
// Images use approximate vision estimates; this is not a provider billing tokenizer.
// shortcut: audio, video and server-side history are excluded; use provider counting for full-context enforcement.
func Count(body []byte, limit int) int {
	var tenths int64
	tokens := func() int { return int((tenths + 9) / 10) }
	add := func(text string) bool {
		for _, r := range text {
			if r <= 0x7f {
				tenths += 3
			} else if unicode.Is(unicode.Han, r) {
				tenths += 6
			} else {
				tenths += 10
			}
			if tokens() > limit {
				return false
			}
		}
		return true
	}
	addTokens := func(n int) bool {
		if n > limit-tokens() {
			tenths = int64(limit)*10 + 1
			return false
		}
		tenths += int64(n) * 10
		return tokens() <= limit
	}
	var walk func(gjson.Result, bool, int) bool
	walk = func(value gjson.Result, schema bool, depth int) bool {
		switch value.Type {
		case gjson.String:
			return add(value.Str)
		case gjson.JSON:
			if depth >= 64 {
				return add(value.Raw)
			}
			if !schema {
				switch value.Get("type").Str {
				case "image", "image_url", "input_image":
					if strings.EqualFold(value.Get("detail").Str, "low") || strings.EqualFold(value.Get("image_url.detail").Str, "low") {
						return addTokens(85)
					}
					width, height := imageDimensions(value)
					if width > 0 && height > 0 {
						// Bound before converting: client-supplied dimensions can overflow integer arithmetic.
						n := 85 + 170*math.Ceil(float64(width)/512)*math.Ceil(float64(height)/512)
						if n > float64(limit) {
							return addTokens(limit + 1)
						}
						return addTokens(int(n))
					}
					return addTokens(765)
				case "input_audio", "audio", "video", "input_video", "input_file", "file":
					return true
				case "document":
					if value.Get("source.type").Str != "text" {
						return true
					}
				}
			}
			keepGoing := true
			value.ForEach(func(key, child gjson.Result) bool {
				if !schema {
					switch key.Str {
					case "inlineData", "inline_data", "fileData", "file_data":
						mime := strings.ToLower(child.Get("mimeType").Str + child.Get("mime_type").Str)
						if strings.HasPrefix(mime, "image/") {
							keepGoing = addTokens(258)
						}
						return keepGoing
					case "encrypted_content", "signature":
						return true
					}
				}
				allText := schema
				switch key.Str {
				case "tools", "functions", "tool_choice", "arguments", "output", "functionResponse", "function_response", "functionCall", "function_call":
					allText = true
				}
				keepGoing = add(key.Str) && walk(child, allText, depth+1)
				return keepGoing
			})
			return keepGoing
		case gjson.Number, gjson.True, gjson.False:
			return add(value.Raw)
		default:
			return true
		}
	}
	// Select model inputs rather than billing options, metadata or binary payloads.
	root := gjson.ParseBytes(body)
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "tools", "functions", "tool_choice", "function_call":
			return walk(value, true, 0)
		case "system", "instructions", "messages", "input", "contents", "content", "systemInstruction", "system_instruction", "prompt", "query", "questions", "instances", "requests", "response":
			return walk(value, false, 0)
		default:
			return true
		}
	})
	return tokens()
}
