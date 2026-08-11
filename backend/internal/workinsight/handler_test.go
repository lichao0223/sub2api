package workinsight

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func workInsightHandlerRouter(service *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Set(string(servermiddleware.ContextKeyUserRole), "admin")
		c.Next()
	})
	handler := NewAdminHandler(service)
	router.GET("/config", handler.GetConfig)
	router.PUT("/config", handler.UpdateConfig)
	return router
}

func TestAdminConfigNeverEchoesTokenAndMapsVersionConflict(t *testing.T) {
	const token = "work-insight-token-canary"
	cfg := DefaultConfig()
	cfg.AnalyzerToken = token
	stored := storedConfig{Config: cfg, AnalyzerTokenCiphertext: "cipher-canary"}
	service := &Service{}
	service.config.Store(&stored)
	router := workInsightHandlerRouter(service)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, http.StatusOK, get.Code)
	require.NotContains(t, get.Body.String(), token)
	require.NotContains(t, get.Body.String(), "cipher-canary")
	require.Contains(t, get.Body.String(), `"analyzer_token_set":true`)

	cfg.ConfigVersion = 99
	cfg.AnalyzerToken = token
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	update := httptest.NewRecorder()
	router.ServeHTTP(update, request)
	require.Equal(t, http.StatusConflict, update.Code)
	require.Contains(t, update.Body.String(), "ai_work_insight_config_conflict")
	require.NotContains(t, update.Body.String(), token)
}
