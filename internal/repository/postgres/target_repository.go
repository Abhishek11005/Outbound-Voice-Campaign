package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/acme/outbound-call-campaign/internal/repository"
)

// CampaignTargetRepository persists campaign call targets.
type CampaignTargetRepository struct {
	db *sqlx.DB
}

// NewCampaignTargetRepository constructs the repository.
func NewCampaignTargetRepository(db *sqlx.DB) *CampaignTargetRepository {
	return &CampaignTargetRepository{db: db}
}

// BulkInsert inserts a batch of targets.
func (r *CampaignTargetRepository) BulkInsert(ctx context.Context, campaignID uuid.UUID, targets []repository.CampaignTargetRecord) error {
	if len(targets) == 0 {
		return nil
	}

	query := `INSERT INTO campaign_targets (
		id, campaign_id, phone_number, payload, state, scheduled_at, last_attempt_at, attempt_count, created_at, updated_at
	) VALUES (:id, :campaign_id, :phone_number, :payload, :state, :scheduled_at, :last_attempt_at, :attempt_count, :created_at, :updated_at)
	ON CONFLICT (id) DO NOTHING`

	rows := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		payload, err := json.Marshal(t.Payload)
		if err != nil {
			return fmt.Errorf("campaign targets: marshal payload: %w", err)
		}
		rows = append(rows, map[string]any{
			"id":             t.ID,
			"campaign_id":    campaignID,
			"phone_number":   t.PhoneNumber,
			"payload":        payload,
			"state":          t.State,
			"scheduled_at":   t.ScheduledAt,
			"last_attempt_at": t.LastAttempt,
			"attempt_count":  t.AttemptCount,
			"created_at":     t.CreatedAt,
			"updated_at":     t.CreatedAt,
		})
	}

	if _, err := r.db.NamedExecContext(ctx, query, rows); err != nil {
		return fmt.Errorf("campaign targets: bulk insert: %w", err)
	}

	return nil
}

// NextBatchForScheduling fetches pending targets for scheduling.
func (r *CampaignTargetRepository) NextBatchForScheduling(ctx context.Context, campaignID uuid.UUID, limit int) ([]repository.CampaignTargetRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.QueryxContext(ctx, `SELECT id, phone_number, payload, state, scheduled_at, last_attempt_at, attempt_count, created_at
		FROM campaign_targets
		WHERE campaign_id = $1 AND state = 'pending'
		ORDER BY created_at ASC
		LIMIT $2`, campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("campaign targets: select for scheduling: %w", err)
	}
	defer rows.Close()

	var results []repository.CampaignTargetRecord
	for rows.Next() {
		var rec targetRecord
		if err := rows.StructScan(&rec); err != nil {
			return nil, fmt.Errorf("campaign targets: scan: %w", err)
		}
		results = append(results, rec.toModel(campaignID))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("campaign targets: rows err: %w", err)
	}

	return results, nil
}

// MarkScheduled marks targets as scheduled.
func (r *CampaignTargetRepository) MarkScheduled(ctx context.Context, campaignID uuid.UUID, targetIDs []uuid.UUID, scheduledAt time.Time) error {
	if len(targetIDs) == 0 {
		return nil
	}

	query := `UPDATE campaign_targets SET state = 'queued', scheduled_at = $1 WHERE campaign_id = $2 AND id = ANY($3)`
	ids := make([]uuid.UUID, len(targetIDs))
	copy(ids, targetIDs)

	if _, err := r.db.ExecContext(ctx, query, scheduledAt, campaignID, ids); err != nil {
		return fmt.Errorf("campaign targets: mark scheduled: %w", err)
	}
	return nil
}

// SetState updates the state for the specified targets.
func (r *CampaignTargetRepository) SetState(ctx context.Context, campaignID uuid.UUID, targetIDs []uuid.UUID, state string) error {
	if len(targetIDs) == 0 {
		return nil
	}
	query := `UPDATE campaign_targets SET state = $1 WHERE campaign_id = $2 AND id = ANY($3)`
	ids := make([]uuid.UUID, len(targetIDs))
	copy(ids, targetIDs)
	if _, err := r.db.ExecContext(ctx, query, state, campaignID, ids); err != nil {
		return fmt.Errorf("campaign targets: set state: %w", err)
	}
	return nil
}

// ListByCampaign lists targets filtered by state.
func (r *CampaignTargetRepository) ListByCampaign(ctx context.Context, campaignID uuid.UUID, limit int, state string) ([]repository.CampaignTargetRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT id, phone_number, payload, state, scheduled_at, last_attempt_at, attempt_count, created_at
		FROM campaign_targets
		WHERE campaign_id = $1`
	args := []any{campaignID}
	if state != "" {
		query += " AND state = $2"
		args = append(args, state)
		query += " ORDER BY created_at ASC LIMIT $3"
		args = append(args, limit)
	} else {
		query += " ORDER BY created_at ASC LIMIT $2"
		args = append(args, limit)
	}

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("campaign targets: list: %w", err)
	}
	defer rows.Close()

	var results []repository.CampaignTargetRecord
	for rows.Next() {
		var rec targetRecord
		if err := rows.StructScan(&rec); err != nil {
			return nil, fmt.Errorf("campaign targets: scan: %w", err)
		}
		results = append(results, rec.toModel(campaignID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("campaign targets: rows err: %w", err)
	}
	return results, nil
}

type targetRecord struct {
	ID          uuid.UUID      `db:"id"`
	PhoneNumber string         `db:"phone_number"`
	Payload     []byte         `db:"payload"`
	State       string         `db:"state"`
	ScheduledAt sql.NullTime   `db:"scheduled_at"`
	LastAttempt sql.NullTime   `db:"last_attempt_at"`
	AttemptCnt  int            `db:"attempt_count"`
	CreatedAt   time.Time      `db:"created_at"`
}

func (r targetRecord) toModel(campaignID uuid.UUID) repository.CampaignTargetRecord {
	var payload map[string]any
	_ = json.Unmarshal(r.Payload, &payload)

	record := repository.CampaignTargetRecord{
		ID:           r.ID,
		CampaignID:   campaignID,
		PhoneNumber:  r.PhoneNumber,
		Payload:      payload,
		State:        r.State,
		AttemptCount: r.AttemptCnt,
		CreatedAt:    r.CreatedAt,
	}
	if r.ScheduledAt.Valid {
		t := r.ScheduledAt.Time
		record.ScheduledAt = &t
	}
	if r.LastAttempt.Valid {
		t := r.LastAttempt.Time
		record.LastAttempt = &t
	}
	return record
}
