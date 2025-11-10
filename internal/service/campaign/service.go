package campaign

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/repository"
	apperrors "github.com/acme/outbound-call-campaign/pkg/errors"
)

// Service orchestrates campaign lifecycle operations.
type Service struct {
	repo          repository.CampaignRepository
	hoursRepo     repository.BusinessHourRepository
	targetRepo    repository.CampaignTargetRepository
	statsRepo     repository.CampaignStatisticsRepository
	defaultConcurrency int
}

// NewService constructs a campaign service.
func NewService(
	repo repository.CampaignRepository,
	hours repository.BusinessHourRepository,
	targets repository.CampaignTargetRepository,
	stats repository.CampaignStatisticsRepository,
	defaultConcurrency int,
) *Service {
	return &Service{
		repo: repo,
		hoursRepo: hours,
		targetRepo: targets,
		statsRepo: stats,
		defaultConcurrency: defaultConcurrency,
	}
}

// CreateCampaignInput captures campaign creation parameters.
type CreateCampaignInput struct {
	Name               string
	Description        string
	TimeZone           string
	MaxConcurrentCalls int
	RetryPolicy        domain.RetryPolicy
	BusinessHours      []BusinessHourInput
	Targets            []TargetInput
}

// BusinessHourInput expresses a business hour window.
type BusinessHourInput struct {
	DayOfWeek time.Weekday
	Start     time.Time
	End       time.Time
}

// TargetInput expresses a campaign target phone number.
type TargetInput struct {
	PhoneNumber string
	Payload     map[string]any
}

// UpdateCampaignInput captures updatable properties.
type UpdateCampaignInput struct {
	ID                 uuid.UUID
	Name               *string
	Description        *string
	MaxConcurrentCalls *int
	RetryPolicy        *domain.RetryPolicy
	BusinessHours      *[]BusinessHourInput
}

// Create provisions a new campaign.
func (s *Service) Create(ctx context.Context, input CreateCampaignInput) (*domain.Campaign, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	campaign := &domain.Campaign{
		ID:                 uuid.New(),
		Name:               input.Name,
		Description:        input.Description,
		TimeZone:           input.TimeZone,
		MaxConcurrentCalls: s.resolveConcurrency(input.MaxConcurrentCalls),
		RetryPolicy:        normalizeRetry(input.RetryPolicy),
		Status:             domain.CampaignStatusPending,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, fmt.Errorf("campaign service: create campaign: %w", err)
	}

	if err := s.hoursRepo.Replace(ctx, campaign.ID, toDomainBusinessHours(input.BusinessHours)); err != nil {
		return nil, fmt.Errorf("campaign service: store business hours: %w", err)
	}

	if err := s.statsRepo.Ensure(ctx, campaign.ID); err != nil {
		return nil, fmt.Errorf("campaign service: ensure stats: %w", err)
	}

	if len(input.Targets) > 0 {
		records := make([]repository.CampaignTargetRecord, 0, len(input.Targets))
		for _, t := range input.Targets {
			records = append(records, repository.CampaignTargetRecord{
				ID:          uuid.New(),
				CampaignID:  campaign.ID,
				PhoneNumber: t.PhoneNumber,
				Payload:     t.Payload,
				State:       "pending",
				CreatedAt:   now,
			})
		}
		if err := s.targetRepo.BulkInsert(ctx, campaign.ID, records); err != nil {
			return nil, fmt.Errorf("campaign service: store targets: %w", err)
		}
	}

	return campaign, nil
}

// Get retrieves a campaign by id including business hours.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Campaign, error) {
	campaign, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	windows, err := s.hoursRepo.List(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("campaign service: list business hours: %w", err)
	}
	campaign.BusinessHours = windows
	return campaign, nil
}

// List returns campaigns.
func (s *Service) List(ctx context.Context, afterID *uuid.UUID, limit int) ([]*domain.Campaign, error) {
	campaigns, err := s.repo.List(ctx, afterID, limit)
	if err != nil {
		return nil, err
	}
	return campaigns, nil
}

// ListByStatus returns campaigns filtered by status with business hours populated.
func (s *Service) ListByStatus(ctx context.Context, status domain.CampaignStatus, limit int) ([]*domain.Campaign, error) {
	campaigns, err := s.repo.ListByStatus(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	for _, c := range campaigns {
		windows, err := s.hoursRepo.List(ctx, c.ID)
		if err != nil {
			return nil, fmt.Errorf("campaign service: list business hours: %w", err)
		}
		c.BusinessHours = windows
	}
	return campaigns, nil
}

// Update modifies campaign metadata.
func (s *Service) Update(ctx context.Context, input UpdateCampaignInput) (*domain.Campaign, error) {
	campaign, err := s.repo.Get(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		campaign.Name = *input.Name
	}
	if input.Description != nil {
		campaign.Description = *input.Description
	}
	if input.MaxConcurrentCalls != nil {
		campaign.MaxConcurrentCalls = s.resolveConcurrency(*input.MaxConcurrentCalls)
	}
	if input.RetryPolicy != nil {
		campaign.RetryPolicy = normalizeRetry(*input.RetryPolicy)
	}

	campaign.UpdatedAt = time.Now().UTC()

	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, err
	}

	if input.BusinessHours != nil {
		if err := s.hoursRepo.Replace(ctx, campaign.ID, toDomainBusinessHours(*input.BusinessHours)); err != nil {
			return nil, fmt.Errorf("campaign service: update business hours: %w", err)
		}
	}

	return campaign, nil
}

// Start transitions a campaign into in-progress state.
func (s *Service) Start(ctx context.Context, id uuid.UUID) error {
	campaign, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}

	if campaign.Status == domain.CampaignStatusInProgress {
		return nil
	}
	if campaign.Status == domain.CampaignStatusCompleted {
		return fmt.Errorf("campaign service: cannot start completed campaign")
	}

	now := time.Now().UTC()
	campaign.Status = domain.CampaignStatusInProgress
	campaign.StartedAt = &now
	if err := s.repo.Update(ctx, campaign); err != nil {
		return err
	}
	return nil
}

// Pause transitions a campaign to paused state.
func (s *Service) Pause(ctx context.Context, id uuid.UUID) error {
	campaign, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	campaign.Status = domain.CampaignStatusPaused
	if err := s.repo.Update(ctx, campaign); err != nil {
		return err
	}
	return nil
}

// Complete marks a campaign as completed.
func (s *Service) Complete(ctx context.Context, id uuid.UUID) error {
	campaign, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	campaign.Status = domain.CampaignStatusCompleted
	campaign.CompletedAt = &now
	if err := s.repo.Update(ctx, campaign); err != nil {
		return err
	}
	return nil
}

// Stats retrieves aggregated statistics.
func (s *Service) Stats(ctx context.Context, id uuid.UUID) (*domain.CampaignStats, error) {
	stats, err := s.statsRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// AddTargets appends targets to a campaign.
func (s *Service) AddTargets(ctx context.Context, campaignID uuid.UUID, targets []TargetInput) error {
	if len(targets) == 0 {
		return nil
	}

	now := time.Now().UTC()
	records := make([]repository.CampaignTargetRecord, 0, len(targets))
	for _, t := range targets {
		records = append(records, repository.CampaignTargetRecord{
			ID:          uuid.New(),
			CampaignID:  campaignID,
			PhoneNumber: t.PhoneNumber,
			Payload:     t.Payload,
			State:       "pending",
			CreatedAt:   now,
		})
	}

	if err := s.targetRepo.BulkInsert(ctx, campaignID, records); err != nil {
		return fmt.Errorf("campaign service: add targets: %w", err)
	}
	return nil
}

func (s *Service) resolveConcurrency(value int) int {
	if value <= 0 {
		return s.defaultConcurrency
	}
	return value
}

func normalizeRetry(policy domain.RetryPolicy) domain.RetryPolicy {
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = 2 * time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 2 * time.Minute
	}
	if policy.MaxDelay < policy.BaseDelay {
		policy.MaxDelay = policy.BaseDelay
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 5
	}
	return policy
}

func toDomainBusinessHours(inputs []BusinessHourInput) []domain.BusinessHourWindow {
	windows := make([]domain.BusinessHourWindow, 0, len(inputs))
	for _, in := range inputs {
		windows = append(windows, domain.BusinessHourWindow{
			DayOfWeek: in.DayOfWeek,
			Start:     in.Start,
			End:       in.End,
		})
	}
	return windows
}

func validateCreateInput(input CreateCampaignInput) error {
	if input.Name == "" {
		return fmt.Errorf("%w: campaign name is required", apperrors.ErrValidation)
	}
	if input.TimeZone == "" {
		return fmt.Errorf("%w: time zone is required", apperrors.ErrValidation)
	}
	if _, err := time.LoadLocation(input.TimeZone); err != nil {
		return fmt.Errorf("%w: invalid time zone %s: %v", apperrors.ErrValidation, input.TimeZone, err)
	}
	for _, bh := range input.BusinessHours {
		if !bh.End.After(bh.Start) {
			return fmt.Errorf("%w: business hour window must have positive duration", apperrors.ErrValidation)
		}
	}
	return nil
}
