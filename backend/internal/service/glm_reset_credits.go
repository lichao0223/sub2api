package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const glmResetCreditsURL = "https://open.bigmodel.cn/api/biz/customer-package-reset/list?targetType=PERSONAL"
const glmUseResetCreditURL = "https://open.bigmodel.cn/api/biz/customer-package-reset/use"

type glmResetCredit struct {
	RecordID  int64     `json:"record_id,omitempty"`
	ResetType string    `json:"reset_type"`
	GrantType string    `json:"grant_type,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type glmResetCreditSnapshot struct {
	AvailableCount int              `json:"available_count"`
	Credits        []glmResetCredit `json:"credits,omitempty"`
}

type GLMResetCreditsRefreshResult struct {
	ResetCredits   *glmResetCreditSnapshot `json:"rate_limit_reset_credits"`
	CachePersisted bool                    `json:"cache_persisted"`
}

func (s *CNProviderQuotaService) loadGLMResetAccount(ctx context.Context, id int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isGLMAPIKeyAccount(account) {
		return nil, infraerrors.New(http.StatusBadRequest, "GLM_RESET_CREDITS_UNSUPPORTED", "Reset credits require a GLM API key account")
	}
	if strings.TrimSpace(account.GetCredential("api_key")) == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "GLM_RESET_CREDITS_NO_KEY", "GLM API key is empty")
	}
	return account, nil
}

func (s *CNProviderQuotaService) glmResetRequest(ctx context.Context, account *Account, method, url string, payload []byte, result any) error {
	target, err := cnValidateProbeURL(s.cfg, url)
	if err != nil {
		return infraerrors.New(http.StatusForbidden, "GLM_RESET_CREDITS_URL_REJECTED", err.Error())
	}
	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, method, target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", strings.TrimSpace(account.GetCredential("api_key")))
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_UNAVAILABLE", "Could not reach GLM reset credits API")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return infraerrors.Newf(http.StatusBadGateway, "GLM_RESET_CREDITS_UNAVAILABLE", "GLM reset credits returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes+1))
	if err != nil || len(body) > cnQuotaMaxBodyBytes || json.Unmarshal(body, result) != nil {
		return infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_INVALID", "GLM returned an invalid reset credit response")
	}
	return nil
}

func (s *CNProviderQuotaService) queryGLMResetCredits(ctx context.Context, account *Account) (*glmResetCreditSnapshot, error) {
	type upstreamCredit struct {
		RecordID   int64  `json:"recordId"`
		GrantType  string `json:"grantType"`
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
	if err := s.glmResetRequest(ctx, account, http.MethodGet, glmResetCreditsURL, nil, &payload); err != nil {
		return nil, err
	}
	invalidResponse := infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_INVALID", "GLM did not return a valid reset credit list")
	if payload.Code != http.StatusOK || !payload.Success || payload.Data.FiveHourResets == nil || payload.Data.WeekResets == nil {
		return nil, invalidResponse
	}
	result := &glmResetCreditSnapshot{}
	now := time.Now()
	// The mainland console returns expiration times without a UTC offset.
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, group := range []struct {
		resetType string
		cards     []upstreamCredit
	}{{"FIVE_HOUR", payload.Data.FiveHourResets}, {"WEEK", payload.Data.WeekResets}} {
		for _, card := range group.cards {
			if !card.Available {
				continue
			}
			expiresAt, err := time.ParseInLocation(time.DateTime, card.ExpireTime, zone)
			if err != nil {
				return nil, invalidResponse
			}
			if expiresAt.After(now) {
				result.Credits = append(result.Credits, glmResetCredit{RecordID: card.RecordID, ResetType: group.resetType, GrantType: card.GrantType, ExpiresAt: expiresAt})
			}
		}
	}
	result.AvailableCount = len(result.Credits)
	return result, nil
}

func (s *CNProviderQuotaService) cacheGLMResetCredits(ctx context.Context, id int64, snapshot *glmResetCreditSnapshot) *GLMResetCreditsRefreshResult {
	err := s.accountRepo.UpdateExtra(ctx, id, map[string]any{"glm_reset_credit_snapshot": snapshot})
	if err != nil {
		slog.Warn("glm_reset_credit_snapshot_persist_failed", "account_id", id, "error", err)
	}
	return &GLMResetCreditsRefreshResult{ResetCredits: snapshot, CachePersisted: err == nil}
}

// RefreshGLMResetCredits queries personal GLM cards without consuming one.
func (s *CNProviderQuotaService) RefreshGLMResetCredits(ctx context.Context, id int64) (*GLMResetCreditsRefreshResult, error) {
	account, err := s.loadGLMResetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.queryGLMResetCredits(ctx, account)
	if err != nil {
		return nil, err
	}
	return s.cacheGLMResetCredits(ctx, id, snapshot), nil
}

// UseGLMResetCredit only spends the card named in the operator's confirmation.
func (s *CNProviderQuotaService) UseGLMResetCredit(ctx context.Context, id, recordID int64, resetType, grantType, requestID string) (*GLMResetCreditsRefreshResult, error) {
	if recordID <= 0 || (resetType != "WEEK" && resetType != "FIVE_HOUR") || grantType == "" || len(grantType) > 64 || strings.TrimSpace(requestID) == "" || len(requestID) > 128 {
		return nil, infraerrors.New(http.StatusBadRequest, "GLM_RESET_CREDITS_INVALID_REQUEST", "Invalid GLM reset credit request")
	}
	account, err := s.loadGLMResetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.queryGLMResetCredits(ctx, account)
	if err != nil {
		return nil, err
	}
	matches := func(card glmResetCredit) bool {
		return card.RecordID == recordID && card.ResetType == resetType && card.GrantType == grantType
	}
	if !slices.ContainsFunc(snapshot.Credits, matches) {
		return nil, infraerrors.New(http.StatusConflict, "GLM_RESET_CREDIT_UNAVAILABLE", "This GLM reset credit is no longer available; refresh the count")
	}
	payload, err := json.Marshal(map[string]any{
		"targetType": "PERSONAL", "resetType": resetType, "recordId": recordID,
		"grantType": grantType, "requestId": requestID,
	})
	if err != nil {
		return nil, err
	}
	var used struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if err := s.glmResetRequest(ctx, account, http.MethodPost, glmUseResetCreditURL, payload, &used); err != nil {
		return nil, err
	}
	if used.Code != http.StatusOK || !used.Success {
		msg := truncate(strings.TrimSpace(used.Msg), 240)
		if msg == "" {
			msg = "GLM reset credit request failed"
		}
		return nil, infraerrors.New(http.StatusBadGateway, "GLM_RESET_CREDITS_FAILED", msg)
	}

	// A failed read-back must not turn a successful consumption into a retry.
	remaining, err := s.queryGLMResetCredits(ctx, account)
	if err == nil {
		// The list can lag briefly behind a successful use.
		remaining.Credits = slices.DeleteFunc(remaining.Credits, matches)
		remaining.AvailableCount = len(remaining.Credits)
	}
	return s.cacheGLMResetCredits(ctx, id, remaining), nil
}
