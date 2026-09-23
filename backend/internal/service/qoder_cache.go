package service

import (
	"context"
	"time"
)

// QoderCache contains the small Redis surface needed by Qoder session/token caching.
type QoderCache interface {
	Get(context.Context, string) (string, error)
	Set(context.Context, string, string, time.Duration) error
	SetNX(context.Context, string, string, time.Duration) (bool, error)
	Del(context.Context, string) error
}
