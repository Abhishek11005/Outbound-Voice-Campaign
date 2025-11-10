package campaign

import (
	"testing"
	"time"
)

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

func TestValidateBusinessHours(t *testing.T) {
	// Test regular business hours (non-midnight crossing)
	input := CreateCampaignInput{
		Name:     "Test Campaign",
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
		t.Fatalf("expected valid business hours to pass, got error: %v", err)
	}

	// Test midnight-crossing business hours
	inputMidnight := CreateCampaignInput{
		Name:     "Test Campaign Midnight",
		TimeZone: "UTC",
		BusinessHours: []BusinessHourInput{
			{
				DayOfWeek: time.Monday,
				Start:     time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC),
				End:       time.Date(0, 1, 1, 2, 0, 0, 0, time.UTC),
			},
		},
	}

	if err := validateCreateInput(inputMidnight); err != nil {
		t.Fatalf("expected midnight-crossing business hours to pass, got error: %v", err)
	}

	// Test invalid zero-duration business hours
	inputZero := CreateCampaignInput{
		Name:     "Test Campaign Zero",
		TimeZone: "UTC",
		BusinessHours: []BusinessHourInput{
			{
				DayOfWeek: time.Monday,
				Start:     time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
				End:       time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
			},
		},
	}

	if err := validateCreateInput(inputZero); err == nil {
		t.Fatalf("expected zero-duration business hours to fail validation")
	}
}
