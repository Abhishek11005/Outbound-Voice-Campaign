package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/domain"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
	"github.com/segmentio/kafka-go"
)

// Scheduler periodically schedules calls respecting business hours.
type Scheduler struct {
	container *app.Container
}

// New constructs a scheduler.
func New(container *app.Container) *Scheduler {
	return &Scheduler{container: container}
}

// Run executes the scheduling loop until cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	cfg := s.container.Config
	interval := cfg.Scheduler.TickInterval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := s.tick(ctx); err != nil && ctx.Err() == nil {
			s.container.Logger.Error("scheduler tick failed", zap.Error(err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) error {
	services := s.container.Services()
	repos := s.container.Repositories()
	callService := services.Call
	// Use the injected targetRepo from the Scheduler struct
	// targetRepo := repos.Targets // REMOVED: Now injected directly
	logger := s.container.Logger
	logger.Info("scheduler: tick started")

	tracer := otel.Tracer("outbound.scheduler")
	sctx, span := tracer.Start(ctx, "scheduler.tick")
	defer span.End()

	// Check for pending retries first - failed calls should be retried before new calls
	hasPendingRetries, err := s.hasPendingRetries(sctx)
	if err != nil {
		span.RecordError(err)
		logger.Warn("scheduler: failed to check pending retries", zap.Error(err))
		// Continue anyway, but log the issue
	}

	logger.Debug("scheduler: checked for pending retries", zap.Bool("has_pending", hasPendingRetries))

	if hasPendingRetries {
		span.SetAttributes(attribute.Bool("retries.pending", true))
		logger.Info("scheduler: skipping new call dispatch due to pending retries - maintaining fairness")
		return nil // Skip this tick to allow retries to be processed first
	}

	nowUTC := time.Now().UTC()
	campaigns, err := services.Campaign.ListByStatus(sctx, domain.CampaignStatusInProgress, s.campaignFetchLimit())
	if err != nil {
		span.RecordError(err)
		return err
	}
	span.SetAttributes(attribute.Int("campaign.count", len(campaigns)))
	logger.Info("scheduler: found campaigns", zap.Int("count", len(campaigns)), zap.Time("now", nowUTC))

	for _, campaign := range campaigns {
		cctx, cspan := tracer.Start(sctx, "scheduler.campaign", trace.WithAttributes(
			attribute.String("campaign.id", campaign.ID.String()),
			attribute.Int("max_concurrency", campaign.MaxConcurrentCalls),
		))

		logger.Debug("scheduler: processing campaign", zap.String("campaign_id", campaign.ID.String()), zap.String("status", string(campaign.Status)))

		if !isWithinBusinessHours(nowUTC, campaign) {
			logger.Debug("scheduler: campaign outside business hours", zap.String("campaign_id", campaign.ID.String()))
			cspan.End()
			continue
		}

		targets, err := repos.Targets.NextBatchForScheduling(cctx, campaign.ID, s.container.Config.Scheduler.MaxBatchSize)
		if err != nil {
			cspan.RecordError(err)
			logger.Error("scheduler: fetch targets", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			cspan.End()
			continue
		}
		cspan.SetAttributes(attribute.Int("targets.fetched", len(targets)))
		logger.Info("scheduler: fetched targets for campaign", zap.String("campaign_id", campaign.ID.String()), zap.Int("target_count", len(targets)), zap.Int("max_batch_size", s.container.Config.Scheduler.MaxBatchSize))
		if len(targets) == 0 {
			cspan.End()
			continue
		}

		ids := make([]uuid.UUID, 0, len(targets))
		for _, t := range targets {
			ids = append(ids, t.ID)
		}

		scheduledAt := time.Now().UTC()
		if err := repos.Targets.MarkScheduled(cctx, campaign.ID, ids, scheduledAt); err != nil {
			cspan.RecordError(err)
			logger.Error("scheduler: mark scheduled", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			cspan.End()
			continue
		}

		var failed []uuid.UUID
		logger.Info("scheduler: dispatching calls", zap.String("campaign_id", campaign.ID.String()), zap.Int("target_count", len(targets)))
		for _, target := range targets {
			input := callsvc.TriggerCallInput{
				CampaignID:  campaign.ID,
				PhoneNumber: target.PhoneNumber,
				Metadata:    target.Payload,
			}
			call, err := callService.TriggerCall(cctx, input)
			if err != nil {
				failed = append(failed, target.ID)
				cspan.RecordError(err)
				logger.Error("scheduler: trigger call failed", zap.Error(err), zap.String("campaign_id", campaign.ID.String()), zap.String("phone", target.PhoneNumber))
			} else {
				logger.Info("scheduler: call triggered", zap.String("campaign_id", campaign.ID.String()), zap.String("call_id", call.ID.String()), zap.String("phone", target.PhoneNumber))
			}
		}

		if len(failed) > 0 {
			if err := repos.Targets.SetState(cctx, campaign.ID, failed, "pending"); err != nil {
				cspan.RecordError(err)
				logger.Error("scheduler: reset failed targets", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			}
		}
		cspan.End()
	}

	return nil
}

// hasPendingRetries checks if any campaigns have recent failures that should be retried first.
// This ensures failed calls are retried before new calls are dispatched, maintaining fairness.
func (s *Scheduler) hasPendingRetries(ctx context.Context) (bool, error) {
	cfg := s.container.Config
	kafkaClient := s.container.Kafka
	logger := s.container.Logger

	// Check each retry topic for pending messages
	for idx, topic := range cfg.Kafka.RetryTopics {
		// Create a temporary reader with a unique consumer group to avoid interfering with retry workers
		// Set CommitInterval to 0 to prevent committing messages and removing them from the topic
		reader := kafkaClient.NewReaderWithConfig(kafka.ReaderConfig{
			Brokers:        cfg.Kafka.Brokers,
			Topic:          topic,
			GroupID:        fmt.Sprintf("scheduler-retry-check-%d", idx),
			StartOffset:    kafka.FirstOffset,
			CommitInterval: 0,  // IMPORTANT: Do not commit messages
			MaxBytes:       10, // Read small chunks to quickly detect pending messages
		})

		// Try to fetch a message with a very short timeout
		fetchCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		msg, err := reader.FetchMessage(fetchCtx)
		cancel()

		// Close reader immediately after use
		reader.Close()

		if err == nil {
			// There is at least one message in this retry topic
			logger.Debug("scheduler: found pending retry messages", zap.String("topic", topic), zap.String("message.key", string(msg.Key)), zap.Int("message.offset", int(msg.Offset)))
			return true, nil
		}

		// If error is not context timeout, there might be an issue
		if err != context.DeadlineExceeded {
			logger.Warn("scheduler: error checking retry topic", zap.String("topic", topic), zap.Error(err))
		} else if err == context.DeadlineExceeded {
			logger.Debug("scheduler: no pending messages in topic (timeout)", zap.String("topic", topic))
		}
	}

	return false, nil
}

func (s *Scheduler) campaignFetchLimit() int {
	cfg := s.container.Config
	limit := cfg.Scheduler.WorkerCount * 10
	if limit <= 0 {
		limit = 100
	}
	// Increase limit to ensure all campaigns are processed
	// TODO: Consider prioritizing campaigns with pending targets
	return limit * 2 // 80 campaigns
}

func isWithinBusinessHours(nowUTC time.Time, campaign *domain.Campaign) bool {
	if len(campaign.BusinessHours) == 0 {
		return true
	}

	loc, err := time.LoadLocation(campaign.TimeZone)
	if err != nil {
		return true
	}

	local := nowUTC.In(loc)
	minuteOfDay := local.Hour()*60 + local.Minute()
	weekday := local.Weekday()

	for _, window := range campaign.BusinessHours {
		start := window.Start.Hour()*60 + window.Start.Minute()
		end := window.End.Hour()*60 + window.End.Minute()

		if end <= start {
			// window spans midnight
			nextDay := (int(window.DayOfWeek) + 1) % 7
			if window.DayOfWeek == weekday && minuteOfDay >= start {
				return true
			}
			if time.Weekday(nextDay) == weekday && minuteOfDay < end {
				return true
			}
			continue
		}

		if window.DayOfWeek != weekday {
			continue
		}

		if minuteOfDay >= start && minuteOfDay < end {
			return true
		}
	}

	return false
}
