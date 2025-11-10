package mock

import (
	"context"
	"math/rand"
	"time"

	"github.com/acme/outbound-call-campaign/internal/config"
	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/queue"
	"github.com/acme/outbound-call-campaign/internal/telephony"
)

// Provider simulates outbound call behaviour.
type Provider struct {
	successRate float64
	timeout     time.Duration
	rng         *rand.Rand
}

// NewProvider constructs a mock provider with deterministic randomness.
func NewProvider(cfg config.CallBridgeConfig) *Provider {
	seed := time.Now().UnixNano()
	return &Provider{
		successRate: 0.8,
		timeout:     cfg.RequestTimeout,
		rng:         rand.New(rand.NewSource(seed)),
	}
}

// PlaceCall simulates a call attempt.
func (p *Provider) PlaceCall(ctx context.Context, msg queue.DispatchMessage) (telephony.Result, error) {
	duration := time.Duration(1+p.rng.Intn(5)) * time.Second

	select {
	case <-ctx.Done():
		return telephony.Result{Status: domain.CallStatusFailed, Duration: duration, Retryable: true, Error: ctx.Err().Error()}, ctx.Err()
	case <-time.After(duration):
	}

	if p.rng.Float64() <= p.successRate {
		return telephony.Result{Status: domain.CallStatusCompleted, Duration: duration}, nil
	}

	retryable := p.rng.Float64() < 0.7
	return telephony.Result{Status: domain.CallStatusFailed, Duration: duration, Retryable: retryable, Error: "simulated failure"}, nil
}
