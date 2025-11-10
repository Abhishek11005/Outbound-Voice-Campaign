package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/repository"
)

// CampaignRepository implements repository.CampaignRepository using PostgreSQL.
type CampaignRepository struct {
	db *sqlx.DB
}

// NewCampaignRepository constructs a new repository.
func NewCampaignRepository(db *sqlx.DB) *CampaignRepository {
	return &CampaignRepository{db: db}
}

// Create inserts a new campaign.
func (r *CampaignRepository) Create(ctx context.Context, campaign *domain.Campaign) error {
	q := `INSERT INTO campaigns (
		id, name, description, time_zone, max_concurrent_calls, status,
		retry_max_attempts, retry_base_delay_ms, retry_max_delay_ms, retry_jitter,
		created_at, updated_at, started_at, completed_at
	) VALUES (
		:id, :name, :description, :time_zone, :max_concurrent_calls, :status,
		:retry_max_attempts, :retry_base_delay_ms, :retry_max_delay_ms, :retry_jitter,
		:created_at, :updated_at, :started_at, :completed_at
	)`

	params := map[string]any{
		"id":                   campaign.ID,
		"name":                 campaign.Name,
		"description":          campaign.Description,
		"time_zone":            campaign.TimeZone,
		"max_concurrent_calls": campaign.MaxConcurrentCalls,
		"status":               campaign.Status,
		"retry_max_attempts":   campaign.RetryPolicy.MaxAttempts,
		"retry_base_delay_ms":  campaign.RetryPolicy.BaseDelay.Milliseconds(),
		"retry_max_delay_ms":   campaign.RetryPolicy.MaxDelay.Milliseconds(),
		"retry_jitter":         campaign.RetryPolicy.Jitter,
		"created_at":           campaign.CreatedAt,
		"updated_at":           campaign.UpdatedAt,
		"started_at":           campaign.StartedAt,
		"completed_at":         campaign.CompletedAt,
	}

	if _, err := r.db.NamedExecContext(ctx, q, params); err != nil {
		return fmt.Errorf("campaign repo: insert: %w", err)
	}

	return nil
}

// Get fetches a campaign by id.
func (r *CampaignRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Campaign, error) {
	q := `SELECT id, name, description, time_zone, max_concurrent_calls, status,
	       retry_max_attempts, retry_base_delay_ms, retry_max_delay_ms, retry_jitter,
	       created_at, updated_at, started_at, completed_at
	  FROM campaigns WHERE id = $1`

	row := r.db.QueryRowxContext(ctx, q, id)
	var record campaignRecord
	if err := row.StructScan(&record); err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("campaign repo: get: %w", err)
	}

	campaign := record.toDomain()
	return &campaign, nil
}

// Update updates campaign metadata.
func (r *CampaignRepository) Update(ctx context.Context, campaign *domain.Campaign) error {
	q := `UPDATE campaigns SET
		name = :name,
		description = :description,
		status = :status,
		time_zone = :time_zone,
		max_concurrent_calls = :max_concurrent_calls,
		retry_max_attempts = :retry_max_attempts,
		retry_base_delay_ms = :retry_base_delay_ms,
		retry_max_delay_ms = :retry_max_delay_ms,
		retry_jitter = :retry_jitter,
		started_at = :started_at,
		completed_at = :completed_at
	 WHERE id = :id`

	params := map[string]any{
		"id":                   campaign.ID,
		"name":                 campaign.Name,
		"description":          campaign.Description,
		"status":               campaign.Status,
		"time_zone":            campaign.TimeZone,
		"max_concurrent_calls": campaign.MaxConcurrentCalls,
		"retry_max_attempts":   campaign.RetryPolicy.MaxAttempts,
		"retry_base_delay_ms":  campaign.RetryPolicy.BaseDelay.Milliseconds(),
		"retry_max_delay_ms":   campaign.RetryPolicy.MaxDelay.Milliseconds(),
		"retry_jitter":         campaign.RetryPolicy.Jitter,
		"started_at":           campaign.StartedAt,
		"completed_at":         campaign.CompletedAt,
	}

	res, err := r.db.NamedExecContext(ctx, q, params)
	if err != nil {
		return fmt.Errorf("campaign repo: update: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("campaign repo: rows affected: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// UpdateStatus updates campaign status.
func (r *CampaignRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.CampaignStatus) error {
	res, err := r.db.ExecContext(ctx, `UPDATE campaigns SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("campaign repo: update status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("campaign repo: rows affected: %w", err)
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// List returns campaigns with optional pagination.
func (r *CampaignRepository) List(ctx context.Context, afterID *uuid.UUID, limit int) ([]*domain.Campaign, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sqlx.Rows
	var err error
	if afterID != nil {
		rows, err = r.db.QueryxContext(ctx, `SELECT id, name, description, time_zone, max_concurrent_calls, status,
			retry_max_attempts, retry_base_delay_ms, retry_max_delay_ms, retry_jitter,
			created_at, updated_at, started_at, completed_at
		FROM campaigns WHERE id > $1 ORDER BY id ASC LIMIT $2`, *afterID, limit)
	} else {
		rows, err = r.db.QueryxContext(ctx, `SELECT id, name, description, time_zone, max_concurrent_calls, status,
			retry_max_attempts, retry_base_delay_ms, retry_max_delay_ms, retry_jitter,
			created_at, updated_at, started_at, completed_at
		FROM campaigns ORDER BY id ASC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("campaign repo: list: %w", err)
	}
	defer rows.Close()

	var results []*domain.Campaign
	for rows.Next() {
		var record campaignRecord
		if err := rows.StructScan(&record); err != nil {
			return nil, fmt.Errorf("campaign repo: scan: %w", err)
		}
		campaign := record.toDomain()
		results = append(results, &campaign)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("campaign repo: rows err: %w", err)
	}

	return results, nil
}

// ListByStatus returns campaigns filtered by status.
func (r *CampaignRepository) ListByStatus(ctx context.Context, status domain.CampaignStatus, limit int) ([]*domain.Campaign, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.QueryxContext(ctx, `SELECT id, name, description, time_zone, max_concurrent_calls, status,
		retry_max_attempts, retry_base_delay_ms, retry_max_delay_ms, retry_jitter,
		created_at, updated_at, started_at, completed_at
		FROM campaigns WHERE status = $1 ORDER BY updated_at ASC LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("campaign repo: list by status: %w", err)
	}
	defer rows.Close()

	var results []*domain.Campaign
	for rows.Next() {
		var record campaignRecord
		if err := rows.StructScan(&record); err != nil {
			return nil, fmt.Errorf("campaign repo: scan: %w", err)
		}
		campaign := record.toDomain()
		results = append(results, &campaign)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("campaign repo: rows err: %w", err)
	}

	return results, nil
}

type campaignRecord struct {
	ID                 uuid.UUID      `db:"id"`
	Name               string         `db:"name"`
	Description        sql.NullString `db:"description"`
	TimeZone           string         `db:"time_zone"`
	MaxConcurrentCalls int            `db:"max_concurrent_calls"`
	Status             string         `db:"status"`
	RetryMaxAttempts   int            `db:"retry_max_attempts"`
	RetryBaseDelayMs   int64          `db:"retry_base_delay_ms"`
	RetryMaxDelayMs    int64          `db:"retry_max_delay_ms"`
	RetryJitter        float64        `db:"retry_jitter"`
	CreatedAt          sql.NullTime   `db:"created_at"`
	UpdatedAt          sql.NullTime   `db:"updated_at"`
	StartedAt          sql.NullTime   `db:"started_at"`
	CompletedAt        sql.NullTime   `db:"completed_at"`
}

func (r campaignRecord) toDomain() domain.Campaign {
	campaign := domain.Campaign{
		ID:                 r.ID,
		Name:               r.Name,
		Description:        r.Description.String,
		TimeZone:           r.TimeZone,
		MaxConcurrentCalls: r.MaxConcurrentCalls,
		Status:             domain.CampaignStatus(r.Status),
		RetryPolicy: domain.RetryPolicy{
			MaxAttempts: r.RetryMaxAttempts,
			BaseDelay:   time.Duration(r.RetryBaseDelayMs) * time.Millisecond,
			MaxDelay:    time.Duration(r.RetryMaxDelayMs) * time.Millisecond,
			Jitter:      r.RetryJitter,
		},
	}

	return campaign
}