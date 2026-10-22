package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type cachedInputTokenLimit struct {
	limit     int
	enabled   bool
	groupIDs  []int64
	expiresAt int64
}

func (c *cachedInputTokenLimit) forGroup(groupID *int64) int {
	if !c.enabled || len(c.groupIDs) > 0 && (groupID == nil || !slices.Contains(c.groupIDs, *groupID)) {
		return 0
	}
	return c.limit
}

func (s *SettingService) parseGatewayInputTokenLimit(values map[string]string) cachedInputTokenLimit {
	result := cachedInputTokenLimit{
		limit: s.defaultGatewayMaxInputTokens(), enabled: !isFalseSettingValue(values[SettingKeyGatewayInputTokenLimitEnabled]),
		groupIDs: []int64{},
	}
	if n, err := strconv.Atoi(strings.TrimSpace(values[SettingKeyGatewayMaxInputTokens])); err == nil && n > 0 {
		result.limit = n
	}
	if raw := values[SettingKeyGatewayInputTokenLimitGroupIDs]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &result.groupIDs); err != nil {
			// Invalid scope must not silently exempt requests from the limit.
			result.groupIDs = nil
		}
	}
	if result.groupIDs == nil || slices.ContainsFunc(result.groupIDs, func(id int64) bool { return id <= 0 }) {
		result.groupIDs = []int64{}
	}
	return result
}

func (s *SettingService) defaultGatewayMaxInputTokens() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.MaxInputTokens > 0 {
		return s.cfg.Gateway.MaxInputTokens
	}
	return config.DefaultGatewayMaxInputTokens
}

// GetGatewayInputTokenLimit returns zero when disabled or outside the selected groups.
// Settings are cached for one minute; local saves refresh them immediately.
func (s *SettingService) GetGatewayInputTokenLimit(ctx context.Context, groupID *int64) int {
	if s == nil || s.settingRepo == nil {
		return s.defaultGatewayMaxInputTokens()
	}
	if cached, ok := s.inputTokenLimitCache.Load().(*cachedInputTokenLimit); ok && time.Now().UnixNano() < cached.expiresAt {
		return cached.forGroup(groupID)
	}
	s.inputTokenLimitMu.Lock()
	defer s.inputTokenLimitMu.Unlock()
	previous, _ := s.inputTokenLimitCache.Load().(*cachedInputTokenLimit)
	if previous != nil && time.Now().UnixNano() < previous.expiresAt {
		return previous.forGroup(groupID)
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{
		SettingKeyGatewayMaxInputTokens, SettingKeyGatewayInputTokenLimitEnabled, SettingKeyGatewayInputTokenLimitGroupIDs,
	})
	ttl := time.Minute
	if err != nil {
		slog.Warn("failed to get gateway input token limit settings", "error", err)
		values = nil
		ttl = 5 * time.Second
	}
	cached := s.parseGatewayInputTokenLimit(values)
	cached.expiresAt = time.Now().Add(ttl).UnixNano()
	s.inputTokenLimitCache.Store(&cached)
	return cached.forGroup(groupID)
}
