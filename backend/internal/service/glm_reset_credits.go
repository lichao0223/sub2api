package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const glmResetCreditsURL = "https://open.bigmodel.cn/api/biz/customer-package-reset/list?targetType=PERSONAL"

type glmResetCredit struct {
	ExpiresAt time.Time `json:"expires_at"`
}

type glmResetCreditSnapshot struct {
	AvailableCount int              `json:"available_count"`
	Credits        []glmResetCredit `json:"credits,omitempty"`
}

type GLMResetCreditsRefreshResult struct {
	ResetCredits   glmResetCreditSnapshot `json:"rate_limit_reset_credits"`
	CachePersisted bool                   `json:"cache_persisted"`
}

// RefreshGLMResetCredits queries personal GLM cards without consuming one.
func (s *CNProviderQuotaService) RefreshGLMResetCredits(ctx context.Context, accountID int64) (*GLMResetCreditsRefreshResult, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !isLegacyCNUsageAccount(account) || !strings.EqualFold(strings.TrimSpace(account.GetExtraString("model_provider")), "glm") {
		return nil, infraerrors.New(http.StatusBadRequest, "GLM_RESET_CREDITS_UNSUPPORTED", "Reset credits require a GLM API key account")
	}
	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GLM_RESET_CREDITS_NO_KEY", "GLM API key is empty")
	}
	target, err := cnValidateProbeURL(s.cfg, glmResetCreditsURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusForbidden, "GLM_RESET_CREDITS_URL_REJECTED", err.Error())
	}
	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_UNAVAILABLE", "Could not query GLM reset credits")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GLM_RESET_CREDITS_UNAVAILABLE", "GLM reset credits returned HTTP %d", resp.StatusCode)
	}
	type upstreamCredit struct {
		Available  bool   `json:"available"`
		ExpireTime string `json:"expireTime"`
	}
	var payload struct {
		Code    int  `json:"code"`
		Success bool `json:"success"`
		Data    struct {
			FiveHourResets []upstreamCredit `json:"fiveHourResets"`
			WeekResets     []upstreamCredit `json:"weekResets"`
		} `json:"data"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes+1))
	invalidResponse := infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_INVALID", "GLM did not return a valid reset credit list")
	if err != nil || len(body) > cnQuotaMaxBodyBytes || json.Unmarshal(body, &payload) != nil ||
		payload.Code != http.StatusOK || !payload.Success || payload.Data.FiveHourResets == nil || payload.Data.WeekResets == nil {
		return nil, invalidResponse
	}
	result := &GLMResetCreditsRefreshResult{}
	now := time.Now()
	// The mainland console returns expiration times without a UTC offset.
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, cards := range [][]upstreamCredit{payload.Data.FiveHourResets, payload.Data.WeekResets} {
		for _, card := range cards {
			if !card.Available {
				continue
			}
			expiresAt, err := time.ParseInLocation(time.DateTime, card.ExpireTime, zone)
			if err != nil {
				return nil, invalidResponse
			}
			if expiresAt.After(now) {
				result.ResetCredits.Credits = append(result.ResetCredits.Credits, glmResetCredit{ExpiresAt: expiresAt})
			}
		}
	}
	result.ResetCredits.AvailableCount = len(result.ResetCredits.Credits)
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{"glm_reset_credit_snapshot": result.ResetCredits}); err != nil {
		slog.Warn("glm_reset_credit_snapshot_persist_failed", "account_id", accountID, "error", err)
	} else {
		result.CachePersisted = true
	}
	return result, nil
}
