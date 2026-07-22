package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type loginIPBlockSettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r loginIPBlockSettingRepo) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	return r.values, nil
}

func TestLoginIPBlockerCountsConsecutiveFailuresAndSupportsPermanentUnblock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	settings := service.NewSettingService(loginIPBlockSettingRepo{values: map[string]string{
		service.SettingKeyLoginIPBlockEnabled:         "true",
		service.SettingKeyLoginIPBlockThreshold:       "2",
		service.SettingKeyLoginIPBlockDurationSeconds: "0",
	}}, nil)
	blocker := NewLoginIPBlocker(rdb, settings)
	status := http.StatusUnauthorized
	router := gin.New()
	router.POST("/login", blocker.Middleware(), func(c *gin.Context) {
		if status == http.StatusOK {
			MarkLoginPasswordVerified(c)
		}
		c.Status(status)
	})

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}

	require.Equal(t, http.StatusUnauthorized, request().Code)
	status = http.StatusOK
	require.Equal(t, http.StatusOK, request().Code, "successful password verification resets the counter")
	status = http.StatusUnauthorized
	require.Equal(t, http.StatusUnauthorized, request().Code)
	require.Equal(t, http.StatusUnauthorized, request().Code)
	require.Equal(t, http.StatusTooManyRequests, request().Code)

	current, err := blocker.ListCurrent(context.Background())
	require.NoError(t, err)
	require.Len(t, current, 1)
	require.True(t, current[0].Permanent)

	require.NoError(t, blocker.Unblock(context.Background(), "203.0.113.10"))
	current, err = blocker.ListCurrent(context.Background())
	require.NoError(t, err)
	require.Empty(t, current)
	history, err := blocker.ListHistory(context.Background())
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Equal(t, "unblocked", history[0].Event)
	require.Equal(t, "blocked", history[1].Event)
}
