package scylla

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
)

// CallStore persists call records in Scylla.
type CallStore struct {
	session *gocql.Session
}

// NewCallStore creates a new call store.
func NewCallStore(session *gocql.Session) *CallStore {
	return &CallStore{session: session}
}

// CreateCall inserts a call record.
func (s *CallStore) CreateCall(ctx context.Context, record *domain.Call) error {
	bucket := bucketDate(record.CreatedAt)
	if err := s.session.Query(`INSERT INTO calls_by_campaign (campaign_id, bucket, call_id, phone_number, status, attempt_count, scheduled_at, last_attempt_at, updated_at, created_at, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.CampaignID.String(), bucket, record.ID.String(), record.PhoneNumber, string(record.Status), record.AttemptCount,
		record.ScheduledAt, record.LastAttemptAt, record.UpdatedAt, record.CreatedAt, nil,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("call store: insert calls_by_campaign: %w", err)
	}

	if err := s.session.Query(`INSERT INTO calls_by_status (campaign_id, status, bucket, call_id, phone_number, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		record.CampaignID.String(), string(record.Status), bucket, record.ID.String(), record.PhoneNumber, record.UpdatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("call store: insert calls_by_status: %w", err)
	}

	return nil
}

// UpdateCallStatus updates the status for a call.
func (s *CallStore) UpdateCallStatus(ctx context.Context, callID uuid.UUID, status domain.CallStatus, attemptCount int, lastError *string) error {
	// Fetch current record to locate partition data.
	call, err := s.GetCall(ctx, callID)
	if err != nil {
		return err
	}

	bucket := bucketDate(call.CreatedAt)
	if err := s.session.Query(`UPDATE calls_by_campaign SET status = ?, attempt_count = ?, last_attempt_at = ?, updated_at = ?, last_error = ?
		WHERE campaign_id = ? AND bucket = ? AND call_id = ?`,
		string(status), attemptCount, time.Now().UTC(), time.Now().UTC(), lastError,
		call.CampaignID.String(), bucket, callID.String(),
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("call store: update calls_by_campaign: %w", err)
	}

	if err := s.session.Query(`UPDATE calls_by_status SET updated_at = ? WHERE campaign_id = ? AND status = ? AND bucket = ? AND call_id = ?`,
		time.Now().UTC(), call.CampaignID.String(), string(call.Status), bucket, callID.String(),
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("call store: update calls_by_status: %w", err)
	}

	if string(call.Status) != string(status) {
		// remove from old status index and insert into new status index
		if err := s.session.Query(`DELETE FROM calls_by_status WHERE campaign_id = ? AND status = ? AND bucket = ? AND call_id = ?`,
			call.CampaignID.String(), string(call.Status), bucket, callID.String(),
		).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("call store: delete old status: %w", err)
		}
		if err := s.session.Query(`INSERT INTO calls_by_status (campaign_id, status, bucket, call_id, phone_number, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			call.CampaignID.String(), string(status), bucket, callID.String(), call.PhoneNumber, time.Now().UTC(),
		).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("call store: insert new status: %w", err)
		}
	}

	return nil
}

// GetCall retrieves a call by ID.
func (s *CallStore) GetCall(ctx context.Context, callID uuid.UUID) (*domain.Call, error) {
	iter := s.session.Query(`SELECT campaign_id, bucket, call_id, phone_number, status, attempt_count, scheduled_at, last_attempt_at, updated_at, created_at, last_error
		FROM calls_by_campaign
		WHERE call_id = ? ALLOW FILTERING`, callID.String()).WithContext(ctx).Iter()

	var (
		campaignIDStr string
		bucket time.Time
		idStr string
		phone string
		status string
		attemptCount int
		scheduled time.Time
		lastAttempt *time.Time
		updated time.Time
		created time.Time
		lastError *string
	)

	if !iter.Scan(&campaignIDStr, &bucket, &idStr, &phone, &status, &attemptCount, &scheduled, &lastAttempt, &updated, &created, &lastError) {
		if err := iter.Close(); err != nil {
			return nil, fmt.Errorf("call store: fetch call close: %w", err)
		}
		return nil, fmt.Errorf("call store: call %s not found", callID)
	}
	iter.Close()

	// Parse UUID strings back to UUID types
	campaignID, err := uuid.Parse(campaignIDStr)
	if err != nil {
		return nil, fmt.Errorf("call store: parse campaign_id: %w", err)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("call store: parse call_id: %w", err)
	}

	call := &domain.Call{
		ID:           id,
		CampaignID:   campaignID,
		PhoneNumber:  phone,
		Status:       domain.CallStatus(status),
		AttemptCount: attemptCount,
		ScheduledAt:  scheduled,
		UpdatedAt:    updated,
		CreatedAt:    created,
		LastError:    lastError,
	}
	call.CreatedAt = bucketDate(bucket)
	if lastAttempt != nil {
		call.LastAttemptAt = lastAttempt
	}
	return call, nil
}

// ListCallsByCampaign lists calls for a campaign with pagination.
func (s *CallStore) ListCallsByCampaign(ctx context.Context, campaignID uuid.UUID, limit int, pagingState []byte) ([]domain.Call, []byte, error) {
	if limit <= 0 {
		limit = 100
	}

	query := s.session.Query(`SELECT bucket, call_id, phone_number, status, attempt_count, scheduled_at, last_attempt_at, updated_at, created_at, last_error
		FROM calls_by_campaign WHERE campaign_id = ?`, campaignID.String()).WithContext(ctx)
	query = query.PageSize(limit)
	if len(pagingState) > 0 {
		query = query.PageState(pagingState)
	}

	iter := query.Iter()
	calls := make([]domain.Call, 0, limit)

	var (
		bucket time.Time
		callIDStr string
		phone string
		status string
		attemptCount int
		scheduled time.Time
		lastAttempt *time.Time
		updated time.Time
		created time.Time
		lastError *string
	)

	for iter.Scan(&bucket, &callIDStr, &phone, &status, &attemptCount, &scheduled, &lastAttempt, &updated, &created, &lastError) {
		callID, err := uuid.Parse(callIDStr)
		if err != nil {
			// Skip this call if UUID parsing fails
			continue
		}

		call := domain.Call{
			ID:           callID,
			CampaignID:   campaignID,
			PhoneNumber:  phone,
			Status:       domain.CallStatus(status),
			AttemptCount: attemptCount,
			ScheduledAt:  scheduled,
			UpdatedAt:    updated,
			CreatedAt:    created,
			LastError:    lastError,
		}
		if lastAttempt != nil {
			call.LastAttemptAt = lastAttempt
		}
		calls = append(calls, call)
	}

	if err := iter.Close(); err != nil {
		return nil, nil, fmt.Errorf("call store: iter close: %w", err)
	}

	nextState := iter.PageState()

	return calls, nextState, nil
}

// AppendAttempt appends a call attempt record.
func (s *CallStore) AppendAttempt(ctx context.Context, attempt domain.CallAttempt) error {
	durationMs := int64(attempt.Duration / time.Millisecond)
	if err := s.session.Query(`INSERT INTO call_attempts (call_id, attempt_number, status, error, created_at, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		attempt.CallID.String(), attempt.AttemptNum, string(attempt.Status), attempt.Error, attempt.CreatedAt, durationMs,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("call store: append attempt: %w", err)
	}
	return nil
}

func bucketDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
