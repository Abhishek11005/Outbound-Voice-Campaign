package call

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/queue"
	"github.com/acme/outbound-call-campaign/internal/service/concurrency"
)

// Worker consumes call dispatch events and triggers the telephony bridge.
type Worker struct {
	container *app.Container
	rng       *rand.Rand
	limiter   *concurrency.Limiter
}

// New creates a new call worker instance.
func New(container *app.Container) *Worker {
	return &Worker{
		container: container,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		limiter:   container.Limiters().Concurrency,
	}
}

// Run starts the worker loop.
func (w *Worker) Run(ctx context.Context) error {
	cfg := w.container.Config
	log.Printf("DEBUG: Call worker starting, reading from topic %s with group %s", cfg.Kafka.CallTopic, cfg.Kafka.ConsumerGroupID)
	reader := w.container.Kafka.NewReader(cfg.Kafka.CallTopic, cfg.Kafka.ConsumerGroupID)
	defer reader.Close()

	for {
		log.Printf("DEBUG: Call worker waiting for message...")
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.container.Logger.Error("call worker: fetch message", zapError(err))
			continue
		}

		if err := w.processMessage(ctx, reader, m); err != nil {
			w.container.Logger.Error("call worker: process", zapError(err))
		}
	}
}

func (w *Worker) processMessage(ctx context.Context, reader *kafka.Reader, m kafka.Message) error {
	log.Printf("DEBUG: Call worker processing message: %s", string(m.Value))
	var dispatch queue.DispatchMessage
	if err := json.Unmarshal(m.Value, &dispatch); err != nil {
		_ = reader.CommitMessages(ctx, m)
		return fmt.Errorf("unmarshal dispatch: %w", err)
	}

	tracer := otel.Tracer("outbound.callworker")
	sctx, span := tracer.Start(ctx, "call.dispatch", trace.WithAttributes(
		attribute.String("call.id", dispatch.CallID.String()),
		attribute.String("campaign.id", dispatch.CampaignID.String()),
		attribute.Int("attempt", dispatch.Attempt),
	))
	defer span.End()

	release, err := w.waitForSlot(sctx, dispatch)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if release != nil {
		defer release()
	}

	cfg := w.container.Config
	provider := w.container.Providers().Telephony
	publisher := w.container.Dispatchers().StatusPublisher

	timeout := cfg.CallBridge.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	callCtx, cancel := context.WithTimeout(sctx, timeout)
	result, callErr := provider.PlaceCall(callCtx, dispatch)
	cancel()

	statusMsg := queue.StatusMessage{
		CallID:           dispatch.CallID,
		CampaignID:       dispatch.CampaignID,
		PhoneNumber:      dispatch.PhoneNumber,
		Status:           string(result.Status),
		Attempt:          dispatch.Attempt,
		MaxAttempts:      dispatch.MaxAttempts,
		Retryable:        result.Retryable && dispatch.Attempt < dispatch.MaxAttempts,
		RetryBaseMs:      dispatch.RetryBaseMs,
		RetryMaxMs:       dispatch.RetryMaxMs,
		RetryJitter:      dispatch.RetryJitter,
		ConcurrencyLimit: dispatch.ConcurrencyLimit,
		Error:            result.Error,
		OccurredAt:       time.Now().UTC(),
		Metadata:         dispatch.Metadata,
	}

	if result.Duration > 0 {
		statusMsg.DurationMs = int64(result.Duration / time.Millisecond)
	}

	if callErr != nil && statusMsg.Error == "" {
		statusMsg.Error = callErr.Error()
		statusMsg.Retryable = dispatch.Attempt < dispatch.MaxAttempts
		statusMsg.Status = string(domain.CallStatusFailed)
		span.RecordError(callErr)
	}

	if statusMsg.Retryable {
		next := w.computeNextAttempt(dispatch)
		statusMsg.NextAttempt = &next
	}

	if err := publisher.PublishStatus(sctx, statusMsg); err != nil {
		span.RecordError(err)
		w.container.Logger.Error("call worker: publish status", zapError(err))
	}

	if err := reader.CommitMessages(sctx, m); err != nil {
		span.RecordError(err)
		return fmt.Errorf("commit message: %w", err)
	}
	return nil
}

func (w *Worker) waitForSlot(ctx context.Context, dispatch queue.DispatchMessage) (func(), error) {
	limiter := w.limiter
	if limiter == nil || dispatch.CampaignID == uuid.Nil {
		return nil, nil
	}

	limit := dispatch.ConcurrencyLimit
	if limit <= 0 {
		limit = w.container.Config.Throttle.DefaultPerCampaign
	}
	if limit <= 0 {
		return nil, nil
	}

	for {
		acquired, err := limiter.Acquire(ctx, dispatch.CampaignID, limit)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if acquired {
			release := func() {
				err := limiter.Release(context.Background(), dispatch.CampaignID)
				if err != nil {
					w.container.Logger.Warn("call worker: release slot", zap.Error(err))
				}
			}
			return release, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (w *Worker) computeNextAttempt(msg queue.DispatchMessage) time.Time {
	base := time.Duration(msg.RetryBaseMs) * time.Millisecond
	if base <= 0 {
		base = 2 * time.Second
	}
	maxDelay := time.Duration(msg.RetryMaxMs) * time.Millisecond
	if maxDelay <= 0 {
		maxDelay = 2 * time.Minute
	}

	exponent := math.Pow(2, float64(msg.Attempt-1))
	delay := time.Duration(exponent) * base
	if delay > maxDelay {
		delay = maxDelay
	}

	if msg.RetryJitter > 0 {
		jitterFraction := w.rng.Float64()*msg.RetryJitter - (msg.RetryJitter / 2)
		jitter := time.Duration(float64(delay) * jitterFraction)
		delay += jitter
		if delay < base {
			delay = base
		}
	}

	return time.Now().UTC().Add(delay)
}

func zapError(err error) zap.Field {
	return zap.Error(err)
}
