package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
	apperrors "github.com/acme/outbound-call-campaign/pkg/errors"
)

var (
	// ErrNotFound indicates the entity was not located.
	ErrNotFound = apperrors.ErrNotFound
	// ErrConflict indicates a unique constraint violation.
	ErrConflict = apperrors.ErrConflict
)

// CampaignRepository manages campaign metadata persistence.
type CampaignRepository interface {
	Create(ctx context.Context, campaign *domain.Campaign) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Campaign, error)
	Update(ctx context.Context, campaign *domain.Campaign) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.CampaignStatus) error
	List(ctx context.Context, afterID *uuid.UUID, limit int) ([]*domain.Campaign, error)
	ListByStatus(ctx context.Context, status domain.CampaignStatus, limit int) ([]*domain.Campaign, error)
}

// BusinessHourRepository manages campaign business hours.
type BusinessHourRepository interface {
	Replace(ctx context.Context, campaignID uuid.UUID, windows []domain.BusinessHourWindow) error
	List(ctx context.Context, campaignID uuid.UUID) ([]domain.BusinessHourWindow, error)
}

// CampaignTargetRepository stores campaign call targets.
type CampaignTargetRepository interface {
	BulkInsert(ctx context.Context, campaignID uuid.UUID, targets []CampaignTargetRecord) error
	NextBatchForScheduling(ctx context.Context, campaignID uuid.UUID, limit int) ([]CampaignTargetRecord, error)
	MarkScheduled(ctx context.Context, campaignID uuid.UUID, targetIDs []uuid.UUID, scheduledAt time.Time) error
	SetState(ctx context.Context, campaignID uuid.UUID, targetIDs []uuid.UUID, state string) error
	ListByCampaign(ctx context.Context, campaignID uuid.UUID, limit int, state string) ([]CampaignTargetRecord, error)
}

// CampaignStatisticsRepository keeps aggregate counters.
type CampaignStatisticsRepository interface {
	Ensure(ctx context.Context, campaignID uuid.UUID) error
	Get(ctx context.Context, campaignID uuid.UUID) (*domain.CampaignStats, error)
	ApplyDelta(ctx context.Context, campaignID uuid.UUID, delta StatsDelta) error
}

// CallStore persists call execution data.
type CallStore interface {
	CreateCall(ctx context.Context, record *domain.Call) error
	UpdateCallStatus(ctx context.Context, callID uuid.UUID, status domain.CallStatus, attemptCount int, lastError *string) error
	GetCall(ctx context.Context, callID uuid.UUID) (*domain.Call, error)
	ListCallsByCampaign(ctx context.Context, campaignID uuid.UUID, limit int, pagingState []byte) ([]domain.Call, []byte, error)
	AppendAttempt(ctx context.Context, attempt domain.CallAttempt) error
}

// CampaignTargetRecord is the storage representation of a campaign target.
type CampaignTargetRecord struct {
	ID           uuid.UUID
	CampaignID   uuid.UUID
	PhoneNumber  string
	Payload      map[string]any
	State        string
	ScheduledAt  *time.Time
	LastAttempt  *time.Time
	AttemptCount int
	CreatedAt    time.Time
}

// StatsDelta captures atomic counter increments.
type StatsDelta struct {
	TotalCallsDelta      int64
	CompletedCallsDelta  int64
	FailedCallsDelta     int64
	InProgressCallsDelta int64
	PendingCallsDelta    int64
	RetriesDelta         int64
}
