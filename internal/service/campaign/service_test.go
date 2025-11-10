package campaign

import (
	"testing"
	"time"
)

func TestNormalizeRetryDefaults(t *testing.T) {
	policy := normalizeRetry(RetryPolicy{})

	if policy.BaseDelay <= 0 {
		t.Fatalf("expected base delay to be set, got %v", policy.BaseDelay)
	}
	if policy.MaxDelay < policy.BaseDelay {
		t.Fatalf("expected max delay >= base delay, got %v < %v", policy.MaxDelay, policy.BaseDelay)
	}
	if policy.MaxAttempts <= 0 {
		t.Fatalf("expected positive max attempts, got %d", policy.MaxAttempts)
	}
}

func TestNormalizeRetryPreservesValues(t *testing.T) {
	policy := normalizeRetry(RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   5 * time.Second,
		MaxDelay:    30 * time.Second,
		Jitter:      0.5,
	})

	if policy.MaxAttempts != 3 {
		t.Errorf("expected max attempts 3, got %d", policy.MaxAttempts)
	}
	if policy.BaseDelay != 5*time.Second {
		t.Errorf("expected base delay 5s, got %v", policy.BaseDelay)
	}
	if policy.MaxDelay != 30*time.Second {
		t.Errorf("expected max delay 30s, got %v", policy.MaxDelay)
	}
	if policy.Jitter != 0.5 {
		t.Errorf("expected jitter 0.5, got %f", policy.Jitter)
	}
}

func TestValidateCreateInputFailures(t *testing.T) {
	cases := []CreateCampaignInput{
		{Name: "", TimeZone: "UTC"},
		{Name: "test", TimeZone: ""},
		{Name: "test", TimeZone: "invalid"},
	}

	for _, tc := range cases {
		if err := validateCreateInput(tc); err == nil {
			t.Errorf("expected validation error for input %+v", tc)
		}
	}
}

func TestValidateCreateInputSuccess(t *testing.T) {
	input := CreateCampaignInput{
		Name:     "test",
		TimeZone: "UTC",
		BusinessHours: []BusinessHourInput{
			{
				DayOfWeek: time.Monday,
				Start:     time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
				End:       time.Date(0, 1, 1, 17, 0, 0, 0, time.UTC),
			},
		},
	}

	if err := validateCreateInput(input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
