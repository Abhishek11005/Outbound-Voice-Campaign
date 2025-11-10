package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/acme/outbound-call-campaign/internal/app"
	"github.com/acme/outbound-call-campaign/internal/domain"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
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
	targetRepo := repos.Targets
	logger := s.container.Logger

	tracer := otel.Tracer("outbound.scheduler")
	sctx, span := tracer.Start(ctx, "scheduler.tick")
	defer span.End()

	nowUTC := time.Now().UTC()
	campaigns, err := services.Campaign.ListByStatus(sctx, domain.CampaignStatusInProgress, s.campaignFetchLimit())
	if err != nil {
		span.RecordError(err)
		return err
	}
	span.SetAttributes(attribute.Int("campaign.count", len(campaigns)))

	for _, campaign := range campaigns {
		cctx, cspan := tracer.Start(sctx, "scheduler.campaign", trace.WithAttributes(
			attribute.String("campaign.id", campaign.ID.String()),
			attribute.Int("max_concurrency", campaign.MaxConcurrentCalls),
		))

		if !isWithinBusinessHours(nowUTC, campaign) {
			cspan.End()
			continue
		}

		targets, err := targetRepo.NextBatchForScheduling(cctx, campaign.ID, s.container.Config.Scheduler.MaxBatchSize)
		if err != nil {
			cspan.RecordError(err)
			logger.Error("scheduler: fetch targets", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			cspan.End()
			continue
		}
		cspan.SetAttributes(attribute.Int("targets.fetched", len(targets)))
		if len(targets) == 0 {
			cspan.End()
			continue
		}

		ids := make([]uuid.UUID, 0, len(targets))
		for _, t := range targets {
			ids = append(ids, t.ID)
		}

		scheduledAt := time.Now().UTC()
		if err := targetRepo.MarkScheduled(cctx, campaign.ID, ids, scheduledAt); err != nil {
			cspan.RecordError(err)
			logger.Error("scheduler: mark scheduled", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			cspan.End()
			continue
		}

		var failed []uuid.UUID
		for _, target := range targets {
			campaignID := campaign.ID
			input := callsvc.TriggerCallInput{
				CampaignID:  &campaignID,
				PhoneNumber: target.PhoneNumber,
				Metadata:    target.Payload,
			}
			if _, err := callService.TriggerCall(cctx, input); err != nil {
				failed = append(failed, target.ID)
				cspan.RecordError(err)
				logger.Error("scheduler: trigger call", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			}
		}

		if len(failed) > 0 {
			if err := targetRepo.SetState(cctx, campaign.ID, failed, "pending"); err != nil {
				cspan.RecordError(err)
				logger.Error("scheduler: reset failed targets", zap.Error(err), zap.String("campaign_id", campaign.ID.String()))
			}
		}
		cspan.End()
	}

	return nil
}

func (s *Scheduler) campaignFetchLimit() int {
	cfg := s.container.Config
	limit := cfg.Scheduler.WorkerCount * 10
	if limit <= 0 {
		limit = 100
	}
	return limit
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
