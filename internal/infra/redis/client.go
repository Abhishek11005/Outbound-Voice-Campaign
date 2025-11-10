package redis

import (
	"context"
	"fmt"

	redis "github.com/redis/go-redis/v9"

	"github.com/acme/outbound-call-campaign/internal/config"
)

// Client wraps a go-redis client.
type Client struct {
	inner *redis.Client
}

// NewClient creates a new redis client from config.
func NewClient(cfg config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return &Client{inner: client}, nil
}

// Close closes the underlying client.
func (c *Client) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

// Inner exposes the raw redis client.
func (c *Client) Inner() *redis.Client {
	return c.inner
}
