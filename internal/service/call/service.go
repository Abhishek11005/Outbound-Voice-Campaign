package call

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
	"github.com/acme/outbound-call-campaign/internal/queue"
	"github.com/acme/outbound-call-campaign/internal/repository"
	"github.com/acme/outbound-call-campaign/internal/service/common"
	apperrors "github.com/acme/outbound-call-campaign/pkg/errors"
)

// Dispatcher is responsible for pushing call dispatch events.
type Dispatcher interface {
	DispatchCall(ctx context.Context, msg queue.DispatchMessage) error
}

// Service coordinates call lifecycle operations.
type Service struct {
	calls              repository.CallStore
	campaigns          repository.CampaignRepository
	targets            repository.CampaignTargetRepository
	stats              repository.CampaignStatisticsRepository
	dispatcher         Dispatcher
	defaultRetry       domain.RetryPolicy
	defaultConcurrency int
}

// NewService builds the call management service.
func NewService(
	store repository.CallStore,
	campaignRepo repository.CampaignRepository,
	targetRepo repository.CampaignTargetRepository,
	statsRepo repository.CampaignStatisticsRepository,
	dispatcher Dispatcher,
	defaultRetry domain.RetryPolicy,
	defaultConcurrency int,
) *Service {
	return &Service{
		calls:              store,
		campaigns:          campaignRepo,
		targets:            targetRepo,
		stats:              statsRepo,
		dispatcher:         dispatcher,
		defaultRetry:       defaultRetry,
		defaultConcurrency: defaultConcurrency,
	}
}

// TriggerCallInput encapsulates the arguments for triggering a call.
type TriggerCallInput struct {
	CampaignID  uuid.UUID
	PhoneNumber string
	Metadata    map[string]any
}

// TriggerCall creates and enqueues a call.
func (s *Service) TriggerCall(ctx context.Context, input TriggerCallInput) (*domain.Call, error) {
	log.Printf("DEBUG: TriggerCall called for campaign %s, phone %s", input.CampaignID, input.PhoneNumber)
	if input.PhoneNumber == "" {
		return nil, fmt.Errorf("%w: phone number is required", apperrors.ErrValidation)
	}

	campaignID := input.CampaignID
	campaign, err := s.campaigns.Get(ctx, campaignID)
	if err != nil {
		log.Printf("DEBUG: Failed to get campaign %s: %v", campaignID, err)
		return nil, fmt.Errorf("call service: lookup campaign: %w", err)
	}
	log.Printf("DEBUG: Got campaign %s", campaign.Name)

	// Validate that the phone number is part of the campaign's registered targets
	if err := s.validatePhoneInCampaignTargets(ctx, campaignID, input.PhoneNumber); err != nil {
		return nil, err
	}

	policy := campaign.RetryPolicy
	concurrencyLimit := s.defaultConcurrency
	if campaign.MaxConcurrentCalls > 0 {
		concurrencyLimit = campaign.MaxConcurrentCalls
	}

	now := time.Now().UTC()
	call := &domain.Call{
		ID:           uuid.New(),
		CampaignID:   campaignID,
		PhoneNumber:  input.PhoneNumber,
		Status:       domain.CallStatusQueued,
		AttemptCount: 0,
		ScheduledAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastError:    nil,
	}

	if err := s.calls.CreateCall(ctx, call); err != nil {
		log.Printf("DEBUG: Failed to create call: %v", err)
		return nil, fmt.Errorf("call service: persist call: %w", err)
	}
	log.Printf("DEBUG: Call created successfully: %s", call.ID)

	delta := repository.StatsDelta{TotalCallsDelta: 1, PendingCallsDelta: 1}
	if err := s.stats.ApplyDelta(ctx, campaignID, delta); err != nil {
		log.Printf("DEBUG: Failed to update stats: %v", err)
		return nil, fmt.Errorf("call service: update stats: %w", err)
	}
	log.Printf("DEBUG: Stats updated successfully")

	payload := queue.DispatchMessage{
		CallID:           call.ID,
		CampaignID:       call.CampaignID,
		PhoneNumber:      call.PhoneNumber,
		Attempt:          1,
		MaxAttempts:      policy.MaxAttempts,
		RetryBaseMs:      policy.BaseDelay.Milliseconds(),
		RetryMaxMs:       policy.MaxDelay.Milliseconds(),
		RetryJitter:      policy.Jitter,
		ConcurrencyLimit: concurrencyLimit,
		Metadata:         input.Metadata,
		EnqueuedAt:       now,
	}

	if err := s.dispatcher.DispatchCall(ctx, payload); err != nil {
		if campaignID != uuid.Nil {
			_ = s.stats.ApplyDelta(ctx, campaignID, repository.StatsDelta{PendingCallsDelta: -1})
		}
		return nil, fmt.Errorf("call service: dispatch call: %w", err)
	}

	return call, nil
}

// validatePhoneInCampaignTargets checks if a phone number is part of the campaign's registered targets.
func (s *Service) validatePhoneInCampaignTargets(ctx context.Context, campaignID uuid.UUID, phoneNumber string) error {
	// Get all existing targets for this campaign to validate against
	existingTargets, err := s.targets.ListByCampaign(ctx, campaignID, 10000, "") // Get all targets, no state filter
	if err != nil {
		return fmt.Errorf("call service: get campaign targets: %w", err)
	}

	// If this campaign has no registered targets, reject the call
	if len(existingTargets) == 0 {
		return fmt.Errorf("%w: campaign has no registered targets", apperrors.ErrValidation)
	}

	// Check if the phone number is in the registered targets
	for _, target := range existingTargets {
		if target.PhoneNumber == phoneNumber {
			return nil // Phone number is valid
		}
	}

	return fmt.Errorf("%w: phone number %s is not part of this campaign's registered target list", apperrors.ErrValidation, phoneNumber)
}

// GetCall retrieves a call by id.
func (s *Service) GetCall(ctx context.Context, id uuid.UUID) (*domain.Call, error) {
	call, err := s.calls.GetCall(ctx, id)
	if err != nil {
		return nil, err
	}
	return call, nil
}

// ListCallsByCampaign lists calls with pagination token.
type ListCallsByCampaignResult struct {
	Calls      []domain.Call
	PagingState []byte
}

func (s *Service) ListCallsByCampaign(ctx context.Context, campaignID uuid.UUID, limit int, pagingState []byte) (*ListCallsByCampaignResult, error) {
	calls, next, err := s.calls.ListCallsByCampaign(ctx, campaignID, limit, pagingState)
	if err != nil {
		return nil, err
	}
	return &ListCallsByCampaignResult{Calls: calls, PagingState: next}, nil
}

// EncodePagingState converts the paging state to base64 for API responses.
func EncodePagingState(state []byte) string {
	if len(state) == 0 {
		return ""
	}
	return common.EncodeBase64(state)
}

// DecodePagingState decodes a base64 token to paging state bytes.
func DecodePagingState(token string) ([]byte, error) {
	if token == "" {
		return nil, nil
	}
	return common.DecodeBase64(token)
}
