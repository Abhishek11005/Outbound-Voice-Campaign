package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/acme/outbound-call-campaign/internal/domain"
)

// BusinessHourRepository persists campaign business hours.
type BusinessHourRepository struct {
	db *sqlx.DB
}

// NewBusinessHourRepository creates a new repository.
func NewBusinessHourRepository(db *sqlx.DB) *BusinessHourRepository {
	return &BusinessHourRepository{db: db}
}

// Replace replaces all business hour windows for a campaign.
func (r *BusinessHourRepository) Replace(ctx context.Context, campaignID uuid.UUID, windows []domain.BusinessHourWindow) error {
	return withTx(ctx, r.db, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM campaign_business_hours WHERE campaign_id = $1`, campaignID); err != nil {
			return fmt.Errorf("business hours: delete existing: %w", err)
		}

		if len(windows) == 0 {
			return nil
		}

		stmt, err := tx.PreparexContext(ctx, `INSERT INTO campaign_business_hours (campaign_id, day_of_week, start_minute, end_minute) VALUES ($1, $2, $3, $4)`)
		if err != nil {
			return fmt.Errorf("business hours: prepare insert: %w", err)
		}
		defer stmt.Close()

		for _, w := range windows {
			start := w.Start.Hour()*60 + w.Start.Minute()
			end := w.End.Hour()*60 + w.End.Minute()
			if _, err := stmt.ExecContext(ctx, campaignID, int(w.DayOfWeek), start, end); err != nil {
				return fmt.Errorf("business hours: insert: %w", err)
			}
		}
		return nil
	})
}

// List retrieves business hours for a campaign.
func (r *BusinessHourRepository) List(ctx context.Context, campaignID uuid.UUID) ([]domain.BusinessHourWindow, error) {
	rows, err := r.db.QueryxContext(ctx, `SELECT day_of_week, start_minute, end_minute FROM campaign_business_hours WHERE campaign_id = $1 ORDER BY day_of_week, start_minute`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("business hours: query: %w", err)
	}
	defer rows.Close()

	var windows []domain.BusinessHourWindow
	for rows.Next() {
		var row struct {
			Day      int `db:"day_of_week"`
			StartMin int `db:"start_minute"`
			EndMin   int `db:"end_minute"`
		}
		if err := rows.StructScan(&row); err != nil {
			return nil, fmt.Errorf("business hours: scan: %w", err)
		}

		windows = append(windows, domain.BusinessHourWindow{
			DayOfWeek: time.Weekday(row.Day),
			Start:     minuteToTime(row.StartMin),
			End:       minuteToTime(row.EndMin),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("business hours: rows err: %w", err)
	}

	return windows, nil
}

func minuteToTime(min int) time.Time {
	hour := min / 60
	minute := min % 60
	return time.Date(2000, time.January, 1, hour, minute, 0, 0, time.UTC)
}
