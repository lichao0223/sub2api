package service

import (
	"testing"
)

func TestTokenRankingModelExcluded(t *testing.T) {
	patterns := []string{"gpt-*", "claude"}
	if !tokenRankingModelExcluded("gpt-5.1", patterns) {
		t.Fatal("expected wildcard match")
	}
	if !tokenRankingModelExcluded("anthropic/claude-sonnet", patterns) {
		t.Fatal("expected contains match")
	}
	if tokenRankingModelExcluded("gemini-2.5", patterns) {
		t.Fatal("unexpected model match")
	}
}
