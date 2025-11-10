package status

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/queue"
	"github.com/acme/outbound-call-campaign/internal/repository"
)

// Worker consumes call status updates and persists them.
type Worker struct {
	container *app.Container
}

// New creates a new status worker.
func New(container *app.Container) *Worker {
	return &Worker{container: container}
}

// Run processes status events until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	cfg := w.container.Config
	groupID := cfg.Kafka.ConsumerGroupID + "-status"
	reader := w.container.Kafka.NewReader(cfg.Kafka.StatusTopic, groupID)
	defer reader.Close()

	repos := w.container.Repositories()
	store := repos.CallStore
	statsRepo := repos.Stats
	retryScheduler := w.container.Dispatchers().RetryScheduler
	logger := w.container.Logger

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("status worker: fetch", zap.Error(err))
			continue
		}

		var status queue.StatusMessage
		if err := json.Unmarshal(msg.Value, &status); err != nil {
			logger.Error("status worker: unmarshal", zap.Error(err))
			_ = reader.CommitMessages(ctx, msg)
			continue
		}

		tracer := otel.Tracer("outbound.statusworker")
		sctx, span := tracer.Start(ctx, "call.status", trace.WithAttributes(
			attribute.String("call.id", status.CallID.String()),
			attribute.String("campaign.id", status.CampaignID.String()),
			attribute.Int("attempt", status.Attempt),
		))
		defer span.End()

		domainStatus := domain.CallStatus(status.Status)
		if err := store.UpdateCallStatus(sctx, status.CallID, domainStatus, status.Attempt, optionalString(status.Error)); err != nil {
			span.RecordError(err)
			logger.Error("status worker: update call", zap.Error(err))
		}

		attempt := domain.CallAttempt{
			ID:         uuid.New(),
			CallID:     status.CallID,
			AttemptNum: status.Attempt,
			Status:     domainStatus,
			Error:      status.Error,
			CreatedAt:  status.OccurredAt,
			Duration:   time.Duration(status.DurationMs) * time.Millisecond,
		}
		if err := store.AppendAttempt(sctx, attempt); err != nil {
			span.RecordError(err)
			logger.Error("status worker: append attempt", zap.Error(err))
		}

		delta := repository.StatsDelta{}
		if status.CampaignID != uuid.Nil {
			if status.Attempt > 1 {
				delta.RetriesDelta++
			}
			switch domainStatus {
			case domain.CallStatusCompleted:
				delta.CompletedCallsDelta++
				delta.PendingCallsDelta--
			case domain.CallStatusFailed:
				if !status.Retryable {
					delta.FailedCallsDelta++
					delta.PendingCallsDelta--
				}
			}

			if err := statsRepo.ApplyDelta(sctx, status.CampaignID, delta); err != nil {
				span.RecordError(err)
				logger.Error("status worker: apply stats", zap.Error(err))
			}
		}

		if status.Retryable && status.NextAttempt != nil {
			retryMsg := queue.RetryMessage{
				DispatchMessage: queue.DispatchMessage{
					CallID:           status.CallID,
					CampaignID:       status.CampaignID,
					PhoneNumber:      status.PhoneNumber,
					Attempt:          status.Attempt + 1,
					MaxAttempts:      status.MaxAttempts,
					RetryBaseMs:      status.RetryBaseMs,
					RetryMaxMs:       status.RetryMaxMs,
					RetryJitter:      status.RetryJitter,
					ConcurrencyLimit: status.ConcurrencyLimit,
					Metadata:         status.Metadata,
					EnqueuedAt:       *status.NextAttempt,
				},
				MaxAttempts: status.MaxAttempts,
				NextAttempt: *status.NextAttempt,
			}
			if err := retryScheduler.ScheduleRetry(sctx, status.Attempt, retryMsg); err != nil {
				span.RecordError(err)
				logger.Error("status worker: schedule retry", zap.Error(err))
			}
		}

		if err := reader.CommitMessages(sctx, msg); err != nil {
			span.RecordError(err)
			logger.Error("status worker: commit", zap.Error(err))
		}
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
