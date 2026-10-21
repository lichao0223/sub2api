package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type cachedInputTokenLimit struct {
	limit     int
	expiresAt int64
}

func (s *SettingService) defaultGatewayMaxInputTokens() int {
	if s != nil && s.cfg != nil && s.cfg.Gateway.MaxInputTokens > 0 {
		return s.cfg.Gateway.MaxInputTokens
	}
	return config.DefaultGatewayMaxInputTokens
}

// GetGatewayMaxInputTokens caches the persisted limit for one minute; local saves refresh it immediately.
func (s *SettingService) GetGatewayMaxInputTokens(ctx context.Context) int {
	if s == nil || s.settingRepo == nil {
		return s.defaultGatewayMaxInputTokens()
	}
	if cached, ok := s.inputTokenLimitCache.Load().(*cachedInputTokenLimit); ok && time.Now().UnixNano() < cached.expiresAt {
		return cached.limit
	}
	s.inputTokenLimitMu.Lock()
	defer s.inputTokenLimitMu.Unlock()
	previous, _ := s.inputTokenLimitCache.Load().(*cachedInputTokenLimit)
	if previous != nil && time.Now().UnixNano() < previous.expiresAt {
		return previous.limit
	}
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyGatewayMaxInputTokens)
	limit, ttl := s.defaultGatewayMaxInputTokens(), time.Minute
	if err == nil {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil && parsed > 0 {
			limit = parsed
		} else {
			ttl = 5 * time.Second
		}
	} else if !errors.Is(err, ErrSettingNotFound) {
		slog.Warn("failed to get gateway_max_input_tokens setting", "error", err)
		ttl = 5 * time.Second
	}
	s.inputTokenLimitCache.Store(&cachedInputTokenLimit{limit: limit, expiresAt: time.Now().Add(ttl).UnixNano()})
	return limit
}
