package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/repository"
)

// CampaignStatisticsRepository implements repository.CampaignStatisticsRepository.
type CampaignStatisticsRepository struct {
	db *sqlx.DB
}

// NewCampaignStatisticsRepository builds the repository.
func NewCampaignStatisticsRepository(db *sqlx.DB) *CampaignStatisticsRepository {
	return &CampaignStatisticsRepository{db: db}
}

// Ensure ensures a row exists for the campaign.
func (r *CampaignStatisticsRepository) Ensure(ctx context.Context, campaignID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO campaign_statistics (campaign_id)
		VALUES ($1) ON CONFLICT (campaign_id) DO NOTHING`, campaignID)
	if err != nil {
		return fmt.Errorf("campaign stats: ensure: %w", err)
	}
	return nil
}

// Get retrieves statistics.
func (r *CampaignStatisticsRepository) Get(ctx context.Context, campaignID uuid.UUID) (*domain.CampaignStats, error) {
	row := r.db.QueryRowxContext(ctx, `SELECT total_calls, completed_calls, failed_calls, in_progress_calls, pending_calls, retries_attempted
		FROM campaign_statistics WHERE campaign_id = $1`, campaignID)

	var stats domain.CampaignStats
	if err := row.StructScan(&stats); err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("campaign stats: get: %w", err)
	}
	return &stats, nil
}

// ApplyDelta applies counter deltas atomically.
func (r *CampaignStatisticsRepository) ApplyDelta(ctx context.Context, campaignID uuid.UUID, delta repository.StatsDelta) error {
	_, err := r.db.ExecContext(ctx, `UPDATE campaign_statistics SET
		total_calls = total_calls + $2,
		completed_calls = completed_calls + $3,
		failed_calls = failed_calls + $4,
		in_progress_calls = in_progress_calls + $5,
		pending_calls = pending_calls + $6,
		retries_attempted = retries_attempted + $7,
		updated_at = NOW()
	WHERE campaign_id = $1`,
		campaignID,
		delta.TotalCallsDelta,
		delta.CompletedCallsDelta,
		delta.FailedCallsDelta,
		delta.InProgressCallsDelta,
		delta.PendingCallsDelta,
		delta.RetriesDelta,
	)
	if err != nil {
		return fmt.Errorf("campaign stats: apply delta: %w", err)
	}
	return nil
}
