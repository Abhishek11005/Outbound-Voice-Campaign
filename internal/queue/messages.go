package queue

import (
	"time"

	"github.com/google/uuid"
)

// DispatchMessage represents an instruction to initiate a call attempt.
type DispatchMessage struct {
	CallID           uuid.UUID         `json:"call_id"`
	CampaignID       uuid.UUID         `json:"campaign_id"`
	PhoneNumber      string            `json:"phone_number"`
	Attempt          int               `json:"attempt"`
	MaxAttempts      int               `json:"max_attempts"`
	RetryBaseMs      int64             `json:"retry_base_ms"`
	RetryMaxMs       int64             `json:"retry_max_ms"`
	RetryJitter      float64           `json:"retry_jitter"`
	ConcurrencyLimit int               `json:"concurrency_limit"`
	Metadata         map[string]any    `json:"metadata"`
	EnqueuedAt       time.Time         `json:"enqueued_at"`
}

// StatusMessage represents the outcome of a call attempt.
type StatusMessage struct {
	CallID           uuid.UUID      `json:"call_id"`
	CampaignID       uuid.UUID      `json:"campaign_id"`
	PhoneNumber      string         `json:"phone_number"`
	Status           string         `json:"status"`
	Attempt          int            `json:"attempt"`
	MaxAttempts      int            `json:"max_attempts"`
	Retryable        bool           `json:"retryable"`
	RetryBaseMs      int64          `json:"retry_base_ms"`
	RetryMaxMs       int64          `json:"retry_max_ms"`
	RetryJitter      float64        `json:"retry_jitter"`
	ConcurrencyLimit int            `json:"concurrency_limit"`
	DurationMs       int64          `json:"duration_ms"`
	Error            string         `json:"error,omitempty"`
	OccurredAt       time.Time      `json:"occurred_at"`
	NextAttempt      *time.Time     `json:"next_attempt,omitempty"`
	Metadata         map[string]any `json:"metadata"`
}

// RetryMessage represents a retry instruction for a failed call.
type RetryMessage struct {
	DispatchMessage
	MaxAttempts  int       `json:"max_attempts"`
	NextAttempt  time.Time `json:"next_attempt"`
}
