package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/acme/outbound-call-campaign/internal/config"
)

// Postgres wraps a sqlx DB instance backed by pgx.
type Postgres struct {
	pool *pgxpool.Pool
	db   *sqlx.DB
}

// NewPostgres creates a new connection pool.
func NewPostgres(ctx context.Context, cfg config.PostgresConfig) (*Postgres, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
	)

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}

	sql := stdlib.OpenDBFromPool(pool)
	db := sqlx.NewDb(sql, "pgx")

	if err := db.PingContext(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return &Postgres{pool: pool, db: db}, nil
}

// DB exposes the sqlx handle.
func (p *Postgres) DB() *sqlx.DB {
	return p.db
}

// Close drains the pool and releases resources.
func (p *Postgres) Close(ctx context.Context) error {
	if p.pool != nil {
		p.pool.Close()
	}
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
