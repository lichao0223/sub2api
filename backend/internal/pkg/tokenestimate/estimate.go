// Package tokenestimate provides a cheap text-only input estimate for admission.
package tokenestimate

import "github.com/tidwall/gjson"

// Count stops at limit+1. Its result is a lower bound when the limit is exceeded.
// ASCII characters count as a quarter token; other Unicode characters count as one.
// shortcut: media and server-side history are excluded; use provider counting for full-context enforcement.
func Count(body []byte, limit int) int {
	var quarters int64
	tokens := func() int { return int((quarters + 3) / 4) }
	add := func(text string) bool {
		for _, r := range text {
			if r <= 0x7f {
				quarters++
			} else {
				quarters += 4
			}
			if tokens() > limit {
				return false
			}
		}
		return true
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
				case "image", "image_url", "input_image", "input_audio", "audio", "video", "input_video", "input_file", "file":
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
					case "inlineData", "inline_data", "fileData", "file_data", "encrypted_content", "signature":
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
