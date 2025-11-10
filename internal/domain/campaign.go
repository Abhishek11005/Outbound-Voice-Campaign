package domain

import (
	"time"

	"github.com/google/uuid"
)

// CampaignStatus enumerates lifecycle states of a campaign.
type CampaignStatus string

const (
	CampaignStatusPending    CampaignStatus = "pending"
	CampaignStatusInProgress CampaignStatus = "in_progress"
	CampaignStatusCompleted  CampaignStatus = "completed"
	CampaignStatusFailed     CampaignStatus = "failed"
	CampaignStatusPaused     CampaignStatus = "paused"
)

// CallStatus enumerates lifecycle stages for an individual call.
type CallStatus string

const (
	CallStatusPending   CallStatus = "pending"
	CallStatusQueued    CallStatus = "queued"
	CallStatusDialing   CallStatus = "dialing"
	CallStatusCompleted CallStatus = "completed"
	CallStatusFailed    CallStatus = "failed"
	CallStatusRetrying  CallStatus = "retrying"
)

// Campaign models an outbound call campaign definition.
type Campaign struct {
	ID                 uuid.UUID
	Name               string
	Description        string
	TimeZone           string
	BusinessHours      []BusinessHourWindow
	MaxConcurrentCalls int
	RetryPolicy        RetryPolicy
	Status             CampaignStatus
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          *time.Time
	CompletedAt        *time.Time
}

// BusinessHourWindow captures allowed calling window per day of week.
type BusinessHourWindow struct {
	DayOfWeek time.Weekday
	Start     time.Time
	End       time.Time
}

// RetryPolicy defines retry rules for failed calls.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      float64
}

// Call represents an individual outbound call within a campaign.
type Call struct {
	ID            uuid.UUID
	CampaignID    uuid.UUID
	PhoneNumber   string
	Status        CallStatus
	AttemptCount  int
	LastAttemptAt *time.Time
	ScheduledAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastError     *string
}

// CallAttempt captures individual call attempts for observability.
type CallAttempt struct {
	ID         uuid.UUID
	CallID     uuid.UUID
	AttemptNum int
	Status     CallStatus
	Error      string
	CreatedAt  time.Time
	Duration   time.Duration
}

// CampaignStats aggregates campaign metrics.
type CampaignStats struct {
	TotalCalls       int64
	CompletedCalls   int64
	FailedCalls      int64
	InProgressCalls  int64
	PendingCalls     int64
	RetriesScheduled int64
}
