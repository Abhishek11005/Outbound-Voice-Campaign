package concurrency

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

// Limiter coordinates campaign-level concurrency using Redis counters.
type Limiter struct {
	client       *redis.Client
	defaultLimit int
	ttl          time.Duration
}

// NewLimiter constructs a concurrency limiter.
func NewLimiter(client *redis.Client, defaultLimit int, ttl time.Duration) *Limiter {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Limiter{client: client, defaultLimit: defaultLimit, ttl: ttl}
}

// Acquire attempts to reserve a slot for the campaign.
func (l *Limiter) Acquire(ctx context.Context, campaignID uuid.UUID, limit int) (bool, error) {
	if campaignID == uuid.Nil {
		return true, nil
	}
	if limit <= 0 {
		limit = l.defaultLimit
	}
	if limit <= 0 {
		return true, nil
	}

	script := redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local current = tonumber(redis.call('GET', key) or '0')
if current < limit then
  current = redis.call('INCR', key)
  if ttl > 0 then
    redis.call('PEXPIRE', key, ttl)
  end
  return 1
end
return 0
`)

	key := l.key(campaignID)
	res, err := script.Run(ctx, l.client, []string{key}, limit, l.ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("concurrency acquire: %w", err)
	}
	return res == 1, nil
}

// Release frees a previously acquired slot.
func (l *Limiter) Release(ctx context.Context, campaignID uuid.UUID) error {
	if campaignID == uuid.Nil {
		return nil
	}
	key := l.key(campaignID)
	script := redis.NewScript(`
local key = KEYS[1]
local current = tonumber(redis.call('GET', key) or '0')
if current <= 0 then
  redis.call('DEL', key)
  return 0
end
return redis.call('DECR', key)
`)
	if _, err := script.Run(ctx, l.client, []string{key}).Int(); err != nil {
		return fmt.Errorf("concurrency release: %w", err)
	}
	return nil
}

func (l *Limiter) key(campaignID uuid.UUID) string {
	return fmt.Sprintf("outbound:campaign:%s:active", campaignID.String())
}
