package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenRankingNonworkRouteSupportsJWTOrAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	h := &handler.Handlers{Usage: handler.NewUsageHandler(nil, nil, nil, nil)}

	v1 := router.Group("/api/v1")
	RegisterUserRoutes(
		v1,
		h,
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			if c.GetHeader("x-test-jwt") != "ok" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
			c.Next()
		}),
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			if c.GetHeader("x-test-admin") != "ok" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
			c.Next()
		}),
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/dashboard/token-ranking/nonwork", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/usage/dashboard/token-ranking/nonwork", nil)
	req.Header.Set("x-api-key", "admin-key")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/usage/dashboard/token-ranking/nonwork", nil)
	req.Header.Set("x-test-jwt", "ok")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/usage/dashboard/token-ranking/nonwork", nil)
	req.Header.Set("x-api-key", "admin-key")
	req.Header.Set("x-test-admin", "ok")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
