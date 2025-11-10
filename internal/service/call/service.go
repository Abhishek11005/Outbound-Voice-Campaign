package call

import (
	"context"
	"fmt"
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
	stats              repository.CampaignStatisticsRepository
	dispatcher         Dispatcher
	defaultRetry       domain.RetryPolicy
	defaultConcurrency int
}

// NewService builds the call management service.
func NewService(
	store repository.CallStore,
	campaignRepo repository.CampaignRepository,
	statsRepo repository.CampaignStatisticsRepository,
	dispatcher Dispatcher,
	defaultRetry domain.RetryPolicy,
	defaultConcurrency int,
) *Service {
	return &Service{
		calls:              store,
		campaigns:          campaignRepo,
		stats:              statsRepo,
		dispatcher:         dispatcher,
		defaultRetry:       defaultRetry,
		defaultConcurrency: defaultConcurrency,
	}
}

// TriggerCallInput encapsulates the arguments for triggering a call.
type TriggerCallInput struct {
	CampaignID  *uuid.UUID
	PhoneNumber string
	Metadata    map[string]any
}

// TriggerCall creates and enqueues a call.
func (s *Service) TriggerCall(ctx context.Context, input TriggerCallInput) (*domain.Call, error) {
	if input.PhoneNumber == "" {
		return nil, fmt.Errorf("%w: phone number is required", apperrors.ErrValidation)
	}

	campaignID := uuid.Nil
	policy := s.defaultRetry
	concurrencyLimit := s.defaultConcurrency
	if input.CampaignID != nil {
		campaignID = *input.CampaignID
		campaign, err := s.campaigns.Get(ctx, campaignID)
		if err != nil {
			return nil, fmt.Errorf("call service: lookup campaign: %w", err)
		}
		policy = campaign.RetryPolicy
		if campaign.MaxConcurrentCalls > 0 {
			concurrencyLimit = campaign.MaxConcurrentCalls
		}
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
		return nil, fmt.Errorf("call service: persist call: %w", err)
	}

	if campaignID != uuid.Nil {
		delta := repository.StatsDelta{TotalCallsDelta: 1, PendingCallsDelta: 1}
		if err := s.stats.ApplyDelta(ctx, campaignID, delta); err != nil {
			return nil, fmt.Errorf("call service: update stats: %w", err)
		}
	}

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
