package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

// MERGE KEEP: local account timezone must affect requests, not just survive in the admin form.
func TestNormalizeOpenAIRequestLocaleAccountTimezone(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","internal_chat_message_metadata_passthrough":{"content_item_kinds":["environments.environment_context","text"]},"content":[{"type":"input_text","text":"<environment_context><timezone>UTC</timezone><current_date>2000-01-01</current_date></environment_context>"},{"type":"input_text","text":"<environment_context><timezone>UTC</timezone><current_date>2000-01-01</current_date></environment_context>"}]}],"tools":[{"type":"web_search","user_location":{"timezone":"UTC"}}]}`)
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, timezone, wantTimezone, wantDate string
	}{
		{"configured", "America/New_York", "America/New_York", "2026-10-19"},
		{"default", "", DefaultOpenAIRequestTimezone, "2026-10-20"},
		{"invalid falls back", "Invalid/Zone", DefaultOpenAIRequestTimezone, "2026-10-20"},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := &Account{Platform: PlatformOpenAI, Extra: map[string]any{openAIRequestTimezoneExtraKey: test.timezone}}
			got := normalizeOpenAIRequestLocale(context.Background(), account, body, "http", now)
			text := gjson.GetBytes(got, "input.0.content.0.text").String()
			if !strings.Contains(text, "<timezone>"+test.wantTimezone+"</timezone>") || !strings.Contains(text, "<current_date>"+test.wantDate+"</current_date>") {
				t.Fatalf("unexpected environment context: %s", text)
			}
			if gjson.GetBytes(got, "tools.0.user_location.timezone").String() != test.wantTimezone {
				t.Fatal("web search timezone does not match account")
			}
			if gjson.GetBytes(got, "input.0.content.1.text").String() != gjson.GetBytes(body, "input.0.content.1.text").String() {
				t.Fatal("ordinary user text was rewritten")
			}
		})
	}
	for _, account := range []*Account{nil, {Platform: PlatformAnthropic}} {
		if got := normalizeOpenAIRequestLocale(context.Background(), account, body, "http", now); string(got) != string(body) {
			t.Fatal("non-OpenAI request was rewritten")
		}
	}
}

func TestValidateOpenAIRequestTimezoneExtra(t *testing.T) {
	for _, test := range []struct {
		name, platform string
		extra          map[string]any
		wantErr        bool
	}{
		{"omitted", PlatformOpenAI, nil, false},
		{"valid", PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: "Asia/Singapore"}, false},
		{"zone outside allowlist", PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: "Asia/Shanghai"}, true},
		{"wrong type", PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: 8}, true},
		{"invalid zone", PlatformOpenAI, map[string]any{openAIRequestTimezoneExtraKey: "Invalid/Zone"}, true},
		{"wrong platform", PlatformAnthropic, map[string]any{openAIRequestTimezoneExtraKey: "Asia/Singapore"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateOpenAIRequestTimezoneExtra(test.platform, test.extra); (err != nil) != test.wantErr {
				t.Fatalf("validation error = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

func TestNormalizeOpenAIRequestAcceptLanguage(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI}
	headers := http.Header{"accept-language": []string{"zh-CN"}}
	normalizeOpenAIRequestAcceptLanguage(account, headers)
	if got := headers.Get("Accept-Language"); got != "en-US,en;q=0.9" {
		t.Fatalf("Accept-Language = %q", got)
	}
}

func TestOpenAIRequestTimezoneOptions(t *testing.T) {
	options := OpenAIRequestTimezoneOptions()
	if len(options) == 0 || !isAllowedOpenAIRequestTimezone(DefaultOpenAIRequestTimezone) {
		t.Fatalf("timezone options do not include the default timezone")
	}
}
