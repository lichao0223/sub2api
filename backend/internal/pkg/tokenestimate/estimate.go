// Package tokenestimate provides a local input count for admission.
package tokenestimate

import (
	"math"
	"strings"

	"github.com/tidwall/gjson"
)

// Count uses the embedded DeepSeek tokenizer for text across all models and stops
// at limit+1. Images use approximate vision estimates.
// shortcut: non-DeepSeek models can tokenize differently; use provider counting for exact model limits.
// shortcut: audio, video and server-side history are excluded; use provider counting for full-context enforcement.
func Count(body []byte, limit int) int {
	total := 0
	addTokens := func(n int) bool {
		if n > limit-total {
			total = limit + 1
			return false
		}
		total += n
		return true
	}
	add := func(text string) bool {
		n, err := deepSeek().count(text, limit-total)
		if err != nil {
			// Fail closed if local tokenization fails rather than bypassing admission.
			total = limit + 1
			return false
		}
		return addTokens(n)
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
	return total
}
