package service

import (
	"context"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
)

const (
	SettingKeyTokenRankingUSDToCNYRate     = "TOKEN_RANKING_USD_TO_CNY_RATE"
	SettingKeyTokenRankingExcludedModels   = "TOKEN_RANKING_EXCLUDED_MODELS"
	SettingKeyTokenRankingExcludedGroupIDs = "TOKEN_RANKING_EXCLUDED_GROUP_IDS"
	SettingKeyUsageDisplayCurrency         = "USAGE_DISPLAY_CURRENCY"
	defaultTokenRankingUSDToCNYRate        = 7.2
	defaultUsageDisplayCurrency            = "CNY"
)

type TokenRankingSettings struct {
	USDToCNYRate     float64  `json:"usd_to_cny_rate"`
	ExcludedModels   []string `json:"excluded_models"`
	ExcludedGroupIDs []int64  `json:"excluded_group_ids"`
}

func (s *SettingService) GetUsageDisplayCurrency(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return defaultUsageDisplayCurrency
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyUsageDisplayCurrency})
	if err != nil {
		return defaultUsageDisplayCurrency
	}
	if strings.EqualFold(strings.TrimSpace(values[SettingKeyUsageDisplayCurrency]), "USD") {
		return "USD"
	}
	return defaultUsageDisplayCurrency
}

func (s *SettingService) UpdateUsageDisplayCurrency(ctx context.Context, currency string) error {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency != "USD" && currency != "CNY" {
		return fmt.Errorf("usage display currency must be USD or CNY")
	}
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting repository unavailable")
	}
	return s.settingRepo.SetMultiple(ctx, map[string]string{SettingKeyUsageDisplayCurrency: currency})
}

func normalizeUsageDisplayCurrency(currency string) string {
	if strings.EqualFold(strings.TrimSpace(currency), "USD") {
		return "USD"
	}
	return defaultUsageDisplayCurrency
}

func (s *SettingService) GetTokenRankingSettings(ctx context.Context) TokenRankingSettings {
	result := TokenRankingSettings{USDToCNYRate: defaultTokenRankingUSDToCNYRate}
	if s == nil || s.settingRepo == nil {
		return result
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyTokenRankingUSDToCNYRate, SettingKeyTokenRankingExcludedModels, SettingKeyTokenRankingExcludedGroupIDs})
	if err != nil {
		return result
	}
	if rate, err := strconv.ParseFloat(strings.TrimSpace(values[SettingKeyTokenRankingUSDToCNYRate]), 64); err == nil && rate > 0 {
		result.USDToCNYRate = rate
	}
	result.ExcludedModels = normalizeTokenRankingPatterns(strings.Split(values[SettingKeyTokenRankingExcludedModels], "\n"))
	result.ExcludedGroupIDs = normalizeTokenRankingGroupIDs(strings.Split(values[SettingKeyTokenRankingExcludedGroupIDs], "\n"))
	return result
}

func (s *SettingService) UpdateTokenRankingSettings(ctx context.Context, rate float64, patterns []string, groupIDs []int64) error {
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("setting repository unavailable")
	}
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("token ranking exchange rate must be positive")
	}
	patterns = normalizeTokenRankingPatterns(patterns)
	groupIDs = normalizeTokenRankingGroupIDsFromInts(groupIDs)
	current := s.GetTokenRankingSettings(ctx)
	updates := map[string]string{SettingKeyTokenRankingUSDToCNYRate: strconv.FormatFloat(rate, 'f', -1, 64)}
	serializedGroupIDs := serializeTokenRankingGroupIDs(groupIDs)
	modelsChanged := strings.Join(patterns, "\n") != strings.Join(current.ExcludedModels, "\n")
	groupsChanged := serializedGroupIDs != serializeTokenRankingGroupIDs(current.ExcludedGroupIDs)
	if modelsChanged || groupsChanged {
		updates[SettingKeyTokenRankingExcludedModels] = strings.Join(patterns, "\n")
		updates[SettingKeyTokenRankingExcludedGroupIDs] = serializedGroupIDs
	}
	return s.settingRepo.SetMultiple(ctx, updates)
}

func normalizeTokenRankingGroupIDs(values []string) []int64 {
	groupIDs := make([]int64, 0, len(values))
	for _, value := range values {
		if groupID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			groupIDs = append(groupIDs, groupID)
		}
	}
	return normalizeTokenRankingGroupIDsFromInts(groupIDs)
}

func normalizeTokenRankingGroupIDsFromInts(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func serializeTokenRankingGroupIDs(values []int64) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatInt(value, 10)
	}
	return strings.Join(parts, "\n")
}

func normalizeTokenRankingPatterns(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func tokenRankingModelExcluded(model string, patterns []string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, pattern := range patterns {
		if strings.ContainsAny(pattern, "*?") {
			if ok, _ := path.Match(pattern, model); ok {
				return true
			}
			continue
		}
		if model == pattern || strings.Contains(model, pattern) {
			return true
		}
	}
	return false
}

func tokenRankingAmountExcluded(model string, groupID int64, settings TokenRankingSettings) bool {
	for _, excludedGroupID := range settings.ExcludedGroupIDs {
		if groupID == excludedGroupID {
			return tokenRankingModelExcluded(model, settings.ExcludedModels)
		}
	}
	return false
}
