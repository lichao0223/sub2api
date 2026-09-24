package service

import (
	"net/http"
	"testing"
)

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
