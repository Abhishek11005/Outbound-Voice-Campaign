package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/acme/outbound-call-campaign/internal/domain"
	campaignsvc "github.com/acme/outbound-call-campaign/internal/service/campaign"
	callsvc "github.com/acme/outbound-call-campaign/internal/service/call"
	apperrors "github.com/acme/outbound-call-campaign/pkg/errors"
)

type createCampaignRequest struct {
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	TimeZone           string                   `json:"time_zone"`
	MaxConcurrentCalls int                      `json:"max_concurrent_calls"`
	RetryPolicy        *retryPolicyRequest      `json:"retry_policy"`
	BusinessHours      []businessHourRequest    `json:"business_hours"`
	Targets            []targetRequest          `json:"targets"`
}

type retryPolicyRequest struct {
	MaxAttempts int     `json:"max_attempts"`
	BaseDelay   string  `json:"base_delay"`
	MaxDelay    string  `json:"max_delay"`
	Jitter      float64 `json:"jitter"`
}

type businessHourRequest struct {
	DayOfWeek int    `json:"day_of_week"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

type targetRequest struct {
	PhoneNumber string                 `json:"phone_number"`
	Metadata    map[string]any         `json:"metadata"`
}

type campaignResponse struct {
	ID                 uuid.UUID               `json:"id"`
	Name               string                  `json:"name"`
	Description        string                  `json:"description"`
	TimeZone           string                  `json:"time_zone"`
	Status             domain.CampaignStatus   `json:"status"`
	MaxConcurrentCalls int                     `json:"max_concurrent_calls"`
	RetryPolicy        retryPolicyResponse     `json:"retry_policy"`
	BusinessHours      []businessHourResponse  `json:"business_hours"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
	StartedAt          *time.Time              `json:"started_at,omitempty"`
	CompletedAt        *time.Time              `json:"completed_at,omitempty"`
}

type retryPolicyResponse struct {
	MaxAttempts int     `json:"max_attempts"`
	BaseDelay   string  `json:"base_delay"`
	MaxDelay    string  `json:"max_delay"`
	Jitter      float64 `json:"jitter"`
}

type businessHourResponse struct {
	DayOfWeek int    `json:"day_of_week"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

type campaignStatsResponse struct {
	TotalCalls       int64 `json:"total_calls"`
	CompletedCalls   int64 `json:"completed_calls"`
	FailedCalls      int64 `json:"failed_calls"`
	InProgressCalls  int64 `json:"in_progress_calls"`
	PendingCalls     int64 `json:"pending_calls"`
	RetriesAttempted int64 `json:"retries_attempted"`
}

type listCampaignsResponse struct {
	Campaigns []campaignResponse `json:"campaigns"`
}

type listCallsResponse struct {
	Calls      []callResponse `json:"calls"`
	NextPage   string         `json:"next_page_token,omitempty"`
}

type callResponse struct {
	ID           uuid.UUID          `json:"id"`
	CampaignID   uuid.UUID          `json:"campaign_id"`
	PhoneNumber  string             `json:"phone_number"`
	Status       domain.CallStatus  `json:"status"`
	AttemptCount int                `json:"attempt_count"`
	ScheduledAt  time.Time          `json:"scheduled_at"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	LastError    *string            `json:"last_error,omitempty"`
}

func (h *HandlerSet) createCampaign(ctx *fiber.Ctx) error {
	var req createCampaignRequest
	if err := ctx.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}

	input, err := h.toCreateCampaignInput(req)
	if err != nil {
		return translateError(err)
	}

	campaign, err := h.campaigns.Create(ctx.Context(), input)
	if err != nil {
		return translateError(err)
	}

	fullCampaign, err := h.campaigns.Get(ctx.Context(), campaign.ID)
	if err != nil {
		return translateError(err)
	}

	return ctx.Status(http.StatusCreated).JSON(toCampaignResponse(fullCampaign))
}

func (h *HandlerSet) listCampaigns(ctx *fiber.Ctx) error {
	limit, _ := strconv.Atoi(ctx.Query("limit", "50"))
	var afterID *uuid.UUID
	if afterStr := ctx.Query("after_id"); afterStr != "" {
		if id, err := uuid.Parse(afterStr); err == nil {
			afterID = &id
		}
	}

	campaigns, err := h.campaigns.List(ctx.Context(), afterID, limit)
	if err != nil {
		return translateError(err)
	}

	resp := listCampaignsResponse{Campaigns: make([]campaignResponse, 0, len(campaigns))}
	for _, c := range campaigns {
		fullCampaign, err := h.campaigns.Get(ctx.Context(), c.ID)
		if err != nil {
			return translateError(err)
		}
		resp.Campaigns = append(resp.Campaigns, toCampaignResponse(fullCampaign))
	}

	return ctx.Status(http.StatusOK).JSON(resp)
}

func (h *HandlerSet) getCampaign(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}

	campaign, err := h.campaigns.Get(ctx.Context(), id)
	if err != nil {
		return translateError(err)
	}

	return ctx.Status(http.StatusOK).JSON(toCampaignResponse(campaign))
}

type updateCampaignRequest struct {
	Name               *string                  `json:"name"`
	Description        *string                  `json:"description"`
	MaxConcurrentCalls *int                     `json:"max_concurrent_calls"`
	RetryPolicy        *retryPolicyRequest      `json:"retry_policy"`
	BusinessHours      *[]businessHourRequest   `json:"business_hours"`
}

func (h *HandlerSet) updateCampaign(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}

	var req updateCampaignRequest
	if err := ctx.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}

	input := campaignsvc.UpdateCampaignInput{ID: id}
	if req.Name != nil {
		input.Name = req.Name
	}
	if req.Description != nil {
		input.Description = req.Description
	}
	if req.MaxConcurrentCalls != nil {
		input.MaxConcurrentCalls = req.MaxConcurrentCalls
	}
	if req.RetryPolicy != nil {
		rp, err := parseRetryPolicy(*req.RetryPolicy)
		if err != nil {
			return translateError(err)
		}
		input.RetryPolicy = &rp
	}
	if req.BusinessHours != nil {
		bh, err := parseBusinessHours(*req.BusinessHours)
		if err != nil {
			return translateError(err)
		}
		input.BusinessHours = &bh
	}

	campaign, err := h.campaigns.Update(ctx.Context(), input)
	if err != nil {
		return translateError(err)
	}

	return ctx.Status(http.StatusOK).JSON(toCampaignResponse(campaign))
}

func (h *HandlerSet) startCampaign(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}
	if err := h.campaigns.Start(ctx.Context(), id); err != nil {
		return translateError(err)
	}
	return ctx.SendStatus(http.StatusNoContent)
}

func (h *HandlerSet) pauseCampaign(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}
	if err := h.campaigns.Pause(ctx.Context(), id); err != nil {
		return translateError(err)
	}
	return ctx.SendStatus(http.StatusNoContent)
}

func (h *HandlerSet) completeCampaign(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}
	if err := h.campaigns.Complete(ctx.Context(), id); err != nil {
		return translateError(err)
	}
	return ctx.SendStatus(http.StatusNoContent)
}

func (h *HandlerSet) campaignStats(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}

	stats, err := h.campaigns.Stats(ctx.Context(), id)
	if err != nil {
		return translateError(err)
	}

	resp := campaignStatsResponse{
		TotalCalls:       stats.TotalCalls,
		CompletedCalls:   stats.CompletedCalls,
		FailedCalls:      stats.FailedCalls,
		InProgressCalls:  stats.InProgressCalls,
		PendingCalls:     stats.PendingCalls,
		RetriesAttempted: stats.RetriesAttempted,
	}

	return ctx.Status(http.StatusOK).JSON(resp)
}

func (h *HandlerSet) addTargets(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}

	var req struct {
		Targets []targetRequest `json:"targets"`
	}
	if err := ctx.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid request body")
	}

	targets := make([]campaignsvc.TargetInput, 0, len(req.Targets))
	for _, t := range req.Targets {
		targets = append(targets, campaignsvc.TargetInput{PhoneNumber: t.PhoneNumber, Payload: t.Metadata})
	}

	if err := h.campaigns.AddTargets(ctx.Context(), id, targets); err != nil {
		return translateError(err)
	}

	return ctx.SendStatus(http.StatusAccepted)
}

func (h *HandlerSet) listCampaignCalls(ctx *fiber.Ctx) error {
	id, err := parseUUID(ctx.Params("id"))
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid campaign id")
	}

	limit, _ := strconv.Atoi(ctx.Query("limit", "100"))
	token := ctx.Query("page_token", "")
	paging, err := callsvc.DecodePagingState(token)
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "invalid page token")
	}

	result, err := h.calls.ListCallsByCampaign(ctx.Context(), id, limit, paging)
	if err != nil {
		return translateError(err)
	}

	resp := listCallsResponse{Calls: make([]callResponse, 0, len(result.Calls))}
	for _, c := range result.Calls {
		resp.Calls = append(resp.Calls, callResponse{
			ID:           c.ID,
			CampaignID:   c.CampaignID,
			PhoneNumber:  c.PhoneNumber,
			Status:       c.Status,
			AttemptCount: c.AttemptCount,
			ScheduledAt:  c.ScheduledAt,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
			LastError:    c.LastError,
		})
	}
	resp.NextPage = callsvc.EncodePagingState(result.PagingState)

	return ctx.Status(http.StatusOK).JSON(resp)
}

func toCampaignResponse(campaign *domain.Campaign) campaignResponse {
	resp := campaignResponse{
		ID:                 campaign.ID,
		Name:               campaign.Name,
		Description:        campaign.Description,
		TimeZone:           campaign.TimeZone,
		Status:             campaign.Status,
		MaxConcurrentCalls: campaign.MaxConcurrentCalls,
		RetryPolicy: retryPolicyResponse{
			MaxAttempts: campaign.RetryPolicy.MaxAttempts,
			BaseDelay:   campaign.RetryPolicy.BaseDelay.String(),
			MaxDelay:    campaign.RetryPolicy.MaxDelay.String(),
			Jitter:      campaign.RetryPolicy.Jitter,
		},
		BusinessHours: make([]businessHourResponse, 0, len(campaign.BusinessHours)),
		CreatedAt:     campaign.CreatedAt,
		UpdatedAt:     campaign.UpdatedAt,
		StartedAt:     campaign.StartedAt,
		CompletedAt:   campaign.CompletedAt,
	}

	for _, window := range campaign.BusinessHours {
		resp.BusinessHours = append(resp.BusinessHours, businessHourResponse{
			DayOfWeek: int(window.DayOfWeek),
			Start:     window.Start.Format("15:04"),
			End:       window.End.Format("15:04"),
		})
	}

	return resp
}

func (h *HandlerSet) toCreateCampaignInput(req createCampaignRequest) (campaignsvc.CreateCampaignInput, error) {
	input := campaignsvc.CreateCampaignInput{
		Name:               req.Name,
		Description:        req.Description,
		TimeZone:           req.TimeZone,
		MaxConcurrentCalls: req.MaxConcurrentCalls,
	}

	if req.RetryPolicy != nil {
		rp, err := parseRetryPolicy(*req.RetryPolicy)
		if err != nil {
			return campaignsvc.CreateCampaignInput{}, err
		}
		input.RetryPolicy = rp
	}

	if len(req.BusinessHours) > 0 {
		windows, err := parseBusinessHours(req.BusinessHours)
		if err != nil {
			return campaignsvc.CreateCampaignInput{}, err
		}
		input.BusinessHours = windows
	}

	targets := make([]campaignsvc.TargetInput, 0, len(req.Targets))
	for _, t := range req.Targets {
		targets = append(targets, campaignsvc.TargetInput{PhoneNumber: t.PhoneNumber, Payload: t.Metadata})
	}
	input.Targets = targets

	return input, nil
}

func parseRetryPolicy(req retryPolicyRequest) (domain.RetryPolicy, error) {
	policy := domain.RetryPolicy{MaxAttempts: req.MaxAttempts, Jitter: req.Jitter}
	if req.BaseDelay != "" {
		d, err := time.ParseDuration(req.BaseDelay)
		if err != nil {
			return domain.RetryPolicy{}, fmt.Errorf("%w: invalid base_delay", apperrors.ErrValidation)
		}
		policy.BaseDelay = d
	}
	if req.MaxDelay != "" {
		d, err := time.ParseDuration(req.MaxDelay)
		if err != nil {
			return domain.RetryPolicy{}, fmt.Errorf("%w: invalid max_delay", apperrors.ErrValidation)
		}
		policy.MaxDelay = d
	}
	return policy, nil
}

func parseBusinessHours(req []businessHourRequest) ([]campaignsvc.BusinessHourInput, error) {
	windows := make([]campaignsvc.BusinessHourInput, 0, len(req))
	for _, bh := range req {
		start, err := time.Parse("15:04", bh.Start)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid start time", apperrors.ErrValidation)
		}
		end, err := time.Parse("15:04", bh.End)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid end time", apperrors.ErrValidation)
		}
		windows = append(windows, campaignsvc.BusinessHourInput{
			DayOfWeek: time.Weekday(bh.DayOfWeek),
			Start:     start,
			End:       end,
		})
	}
	return windows, nil
}

func parseUUID(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}
