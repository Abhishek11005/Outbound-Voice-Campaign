package telephony

import (
	"context"
	"time"

	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/queue"
)

// Result captures the outcome of a telephony attempt.
type Result struct {
	Status     domain.CallStatus
	Duration   time.Duration
	Retryable  bool
	Error      string
}

// Provider abstracts the telephony integration.
type Provider interface {
	PlaceCall(ctx context.Context, msg queue.DispatchMessage) (Result, error)
}
