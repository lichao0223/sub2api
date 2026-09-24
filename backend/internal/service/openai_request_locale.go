package service

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var openAIEnvironmentTimezonePattern = regexp.MustCompile(`(?s)(<environment_context>.*?<timezone>)[^<]*(</timezone>.*?</environment_context>)`)
var openAIEnvironmentDatePattern = regexp.MustCompile(`(?s)(<environment_context>.*?<current_date>)[^<]*(</current_date>.*?</environment_context>)`)

func normalizeOpenAIRequestLocale(_ context.Context, account *Account, body []byte, _ string, now time.Time) []byte {
	if account == nil || !account.IsOpenAI() {
		return body
	}
	timezone := account.OpenAIRequestTimezone()
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return body
	}
	date := now.In(location).Format("2006-01-02")
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" || !item.Get("internal_chat_message_metadata_passthrough.content_item_kinds").IsArray() || !item.Get("content").IsArray() {
				continue
			}
			kinds := item.Get("internal_chat_message_metadata_passthrough.content_item_kinds").Array()
			for contentIndex, part := range item.Get("content").Array() {
				if contentIndex >= len(kinds) || kinds[contentIndex].String() != "environments.environment_context" || part.Get("type").String() != "input_text" {
					continue
				}
				text := part.Get("text")
				if text.Type != gjson.String {
					continue
				}
				value := openAIEnvironmentTimezonePattern.ReplaceAllString(text.String(), `${1}`+timezone+`${2}`)
				value = openAIEnvironmentDatePattern.ReplaceAllString(value, `${1}`+date+`${2}`)
				if value != text.String() {
					if updated, setErr := sjson.SetBytes(body, "input."+strconv.Itoa(inputIndex)+".content."+strconv.Itoa(contentIndex)+".text", value); setErr == nil {
						body = updated
					}
				}
			}
		}
	}
	for index, tool := range gjson.GetBytes(body, "tools").Array() {
		if !strings.HasPrefix(tool.Get("type").String(), "web_search") || tool.Get("user_location.timezone").Type != gjson.String {
			continue
		}
		if updated, setErr := sjson.SetBytes(body, "tools."+strconv.Itoa(index)+".user_location.timezone", timezone); setErr == nil {
			body = updated
		}
	}
	return body
}

func normalizeOpenAIRequestAcceptLanguage(account *Account, headers http.Header) {
	if account == nil || !account.IsOpenAI() || headers == nil {
		return
	}
	found := false
	for name := range headers {
		if strings.EqualFold(name, "Accept-Language") {
			found = true
			delete(headers, name)
		}
	}
	if found {
		headers.Set("Accept-Language", "en-US,en;q=0.9")
	}
}
