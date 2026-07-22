package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	loginIPBlockPrefix     = "login_ip_block:blocked:"
	loginIPFailurePrefix   = "login_ip_block:failures:"
	loginIPBlockHistoryKey = "login_ip_block:history"
	loginIPBlockHistoryMax = 1000
	loginPasswordVerified  = "login_password_verified"
)

var recordLoginFailureScript = redis.NewScript(`
local failures = redis.call('INCR', KEYS[1])
if failures < tonumber(ARGV[1]) then
  return 0
end
redis.call('DEL', KEYS[1])
if tonumber(ARGV[2]) == 0 then
  redis.call('SET', KEYS[2], ARGV[3])
else
  redis.call('SET', KEYS[2], ARGV[3], 'EX', ARGV[2])
end
redis.call('LPUSH', KEYS[3], ARGV[4])
redis.call('LTRIM', KEYS[3], 0, ARGV[5] - 1)
return 1
`)

type LoginIPBlocker struct {
	redis    *redis.Client
	settings *service.SettingService
}

type LoginIPBlockRecord struct {
	IP               string     `json:"ip"`
	BlockedAt        time.Time  `json:"blocked_at"`
	DurationSeconds  int        `json:"duration_seconds"`
	Permanent        bool       `json:"permanent"`
	RemainingSeconds int64      `json:"remaining_seconds,omitempty"`
	Event            string     `json:"event,omitempty"`
	UnblockedAt      *time.Time `json:"unblocked_at,omitempty"`
}

func MarkLoginPasswordVerified(c *gin.Context) {
	if c != nil {
		c.Set(loginPasswordVerified, true)
	}
}

func NewLoginIPBlocker(redisClient *redis.Client, settingService *service.SettingService) *LoginIPBlocker {
	return &LoginIPBlocker{redis: redisClient, settings: settingService}
}

func (b *LoginIPBlocker) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if b == nil || b.redis == nil || b.settings == nil {
			c.Next()
			return
		}
		config, err := b.settings.GetLoginIPBlockConfig(c.Request.Context())
		if err != nil || !config.Enabled {
			if err != nil {
				log.Printf("[LoginIPBlock] load settings failed: %v", err)
			}
			c.Next()
			return
		}

		clientIP := strings.TrimSpace(c.ClientIP())
		if clientIP == "" {
			c.Next()
			return
		}
		blockKey := loginIPBlockPrefix + clientIP
		if record, ttl, blocked, err := b.currentBlock(c.Request.Context(), blockKey); err != nil {
			log.Printf("[LoginIPBlock] check failed for %s: %v", clientIP, err)
		} else if blocked {
			abortLoginIPBlocked(c, record.Permanent, ttl)
			return
		}

		c.Next()
		verified, _ := c.Get(loginPasswordVerified)
		if verified == true || (c.Writer.Status() >= 200 && c.Writer.Status() < 300) {
			if err := b.redis.Del(c.Request.Context(), loginIPFailurePrefix+clientIP).Err(); err != nil {
				log.Printf("[LoginIPBlock] clear failures failed for %s: %v", clientIP, err)
			}
			return
		}
		if c.Writer.Status() != http.StatusUnauthorized {
			return
		}
		if _, err := b.recordFailure(c.Request.Context(), clientIP, config); err != nil {
			log.Printf("[LoginIPBlock] record failure failed for %s: %v", clientIP, err)
		}
	}
}

func (b *LoginIPBlocker) recordFailure(ctx context.Context, ip string, config service.LoginIPBlockConfig) (bool, error) {
	now := time.Now().UTC()
	record := LoginIPBlockRecord{
		IP: ip, BlockedAt: now, DurationSeconds: config.DurationSeconds,
		Permanent: config.DurationSeconds == 0, Event: "blocked",
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	result, err := recordLoginFailureScript.Run(ctx, b.redis, []string{
		loginIPFailurePrefix + ip,
		loginIPBlockPrefix + ip,
		loginIPBlockHistoryKey,
	}, config.Threshold, config.DurationSeconds, string(encoded), string(encoded), loginIPBlockHistoryMax).Int()
	return result == 1, err
}

func (b *LoginIPBlocker) currentBlock(ctx context.Context, key string) (LoginIPBlockRecord, time.Duration, bool, error) {
	value, err := b.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return LoginIPBlockRecord{}, 0, false, nil
	}
	if err != nil {
		return LoginIPBlockRecord{}, 0, false, err
	}
	var record LoginIPBlockRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return LoginIPBlockRecord{}, 0, false, err
	}
	ttl, err := b.redis.TTL(ctx, key).Result()
	if err != nil {
		return LoginIPBlockRecord{}, 0, false, err
	}
	if !record.Permanent && ttl <= 0 {
		return LoginIPBlockRecord{}, 0, false, nil
	}
	return record, ttl, true, nil
}

func (b *LoginIPBlocker) ListCurrent(ctx context.Context) ([]LoginIPBlockRecord, error) {
	if b == nil || b.redis == nil {
		return nil, fmt.Errorf("redis is unavailable")
	}
	items := make([]LoginIPBlockRecord, 0)
	var cursor uint64
	for {
		keys, next, err := b.redis.Scan(ctx, cursor, loginIPBlockPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			record, ttl, blocked, err := b.currentBlock(ctx, key)
			if err != nil {
				return nil, err
			}
			if blocked {
				if !record.Permanent {
					record.RemainingSeconds = max(1, int64(ttl.Seconds()))
				}
				items = append(items, record)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].BlockedAt.After(items[j].BlockedAt) })
	return items, nil
}

func (b *LoginIPBlocker) ListHistory(ctx context.Context) ([]LoginIPBlockRecord, error) {
	if b == nil || b.redis == nil {
		return nil, fmt.Errorf("redis is unavailable")
	}
	values, err := b.redis.LRange(ctx, loginIPBlockHistoryKey, 0, loginIPBlockHistoryMax-1).Result()
	if err != nil {
		return nil, err
	}
	items := make([]LoginIPBlockRecord, 0, len(values))
	for _, value := range values {
		var record LoginIPBlockRecord
		if json.Unmarshal([]byte(value), &record) == nil {
			items = append(items, record)
		}
	}
	return items, nil
}

func (b *LoginIPBlocker) Unblock(ctx context.Context, ip string) error {
	if b == nil || b.redis == nil {
		return fmt.Errorf("redis is unavailable")
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("IP is required")
	}
	if err := b.redis.Del(ctx, loginIPBlockPrefix+ip, loginIPFailurePrefix+ip).Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	record := LoginIPBlockRecord{IP: ip, Event: "unblocked", UnblockedAt: &now}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	pipe := b.redis.Pipeline()
	pipe.LPush(ctx, loginIPBlockHistoryKey, encoded)
	pipe.LTrim(ctx, loginIPBlockHistoryKey, 0, loginIPBlockHistoryMax-1)
	_, err = pipe.Exec(ctx)
	return err
}

func abortLoginIPBlocked(c *gin.Context, permanent bool, ttl time.Duration) {
	retryAfter := int64(0)
	if !permanent {
		retryAfter = max(1, int64(ttl.Seconds()))
		c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": "login IP blocked", "code": "LOGIN_IP_BLOCKED",
		"message":   "Too many consecutive failed login attempts from this IP",
		"permanent": permanent, "retry_after_seconds": retryAfter,
	})
}
