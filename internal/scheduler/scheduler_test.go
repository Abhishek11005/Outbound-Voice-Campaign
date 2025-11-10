package scheduler

import (
	"testing"
	"time"

	"github.com/acme/outbound-call-campaign/internal/domain"
)

func TestIsWithinBusinessHours(t *testing.T) {
	campaign := &domain.Campaign{
		TimeZone: "UTC",
		BusinessHours: []domain.BusinessHourWindow{
			{
				DayOfWeek: time.Monday,
				Start:     time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC),
				End:       time.Date(0, 1, 1, 17, 0, 0, 0, time.UTC),
			},
		},
	}

	mondayMorning := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if !isWithinBusinessHours(mondayMorning, campaign) {
		t.Fatalf("expected %v to be within business hours", mondayMorning)
	}

	mondayNight := time.Date(2024, 1, 1, 20, 0, 0, 0, time.UTC)
	if isWithinBusinessHours(mondayNight, campaign) {
		t.Fatalf("expected %v to be outside business hours", mondayNight)
	}

	tuesdayMorning := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	if isWithinBusinessHours(tuesdayMorning, campaign) {
		t.Fatalf("expected %v to be outside business hours (wrong day)", tuesdayMorning)
	}
}

func TestIsWithinBusinessHoursSpanningMidnight(t *testing.T) {
	campaign := &domain.Campaign{
		TimeZone: "UTC",
		BusinessHours: []domain.BusinessHourWindow{
			{
				DayOfWeek: time.Monday,
				Start:     time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC),
				End:       time.Date(0, 1, 1, 2, 0, 0, 0, time.UTC),
			},
		},
	}

	night := time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC)
	if !isWithinBusinessHours(night, campaign) {
		t.Fatalf("expected %v to be within cross-midnight window", night)
	}

	earlyMorning := time.Date(2024, 1, 2, 1, 0, 0, 0, time.UTC)
	if !isWithinBusinessHours(earlyMorning, campaign) {
		t.Fatalf("expected %v to be within cross-midnight window", earlyMorning)
	}
}
