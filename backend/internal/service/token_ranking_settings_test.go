package service

import (
	"reflect"
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

func TestTokenRankingAmountExcludedByGroup(t *testing.T) {
	settings := TokenRankingSettings{
		ExcludedModels:   []string{"deepseek-*"},
		ExcludedGroupIDs: []int64{12, 15},
	}
	tests := []struct {
		name    string
		model   string
		groupID int64
		want    bool
	}{
		{name: "selected group matching model", model: "deepseek-chat", groupID: 12, want: true},
		{name: "unselected group matching model", model: "deepseek-chat", groupID: 18, want: false},
		{name: "selected group nonmatching model", model: "claude-sonnet", groupID: 12, want: false},
		{name: "missing group", model: "deepseek-chat", groupID: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := tokenRankingAmountExcluded(test.model, test.groupID, settings); got != test.want {
				t.Fatalf("tokenRankingAmountExcluded() = %v, want %v", got, test.want)
			}
		})
	}

	if tokenRankingAmountExcluded("deepseek-chat", 12, TokenRankingSettings{ExcludedModels: settings.ExcludedModels}) {
		t.Fatal("empty group selection must not exclude amounts")
	}
}

func TestNormalizeTokenRankingGroupIDs(t *testing.T) {
	want := []int64{12, 15}
	if got := normalizeTokenRankingGroupIDs([]string{"12", "", "invalid", "12", "-1", "15"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTokenRankingGroupIDs() = %v, want %v", got, want)
	}
}
