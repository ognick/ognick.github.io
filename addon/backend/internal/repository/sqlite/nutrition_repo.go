package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ognick/zabkiss/internal/domain"
)

const nutritionDBPath = "/data/nutrition.db"

type NutritionRepo struct {
	db *sql.DB
}

func NewNutritionRepo() (*NutritionRepo, error) {
	db, err := sql.Open("sqlite", nutritionDBPath)
	if err != nil {
		return nil, fmt.Errorf("open nutrition db: %w", err)
	}
	db.SetMaxOpenConns(1)

	r := &NutritionRepo{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}

func (r *NutritionRepo) migrate() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS food_logs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    TEXT NOT NULL,
			name       TEXT NOT NULL,
			image_ref  TEXT NOT NULL DEFAULT '',
			calories   INTEGER NOT NULL DEFAULT 0,
			protein    INTEGER NOT NULL DEFAULT 0,
			fat        INTEGER NOT NULL DEFAULT 0,
			carbs      INTEGER NOT NULL DEFAULT 0,
			caption    TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_food_logs_user_date ON food_logs(user_id, created_at);
	`)
	return err
}

func (r *NutritionRepo) Close() error {
	return r.db.Close()
}

func (r *NutritionRepo) Save(ctx context.Context, log domain.FoodLog) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO food_logs (user_id, name, image_ref, calories, protein, fat, carbs, caption)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		log.UserID, log.Name, log.ImageRef, log.Calories, log.Protein, log.Fat, log.Carbs, log.Caption,
	)
	if err != nil {
		return fmt.Errorf("save food log: %w", err)
	}
	return nil
}

func (r *NutritionRepo) GetDailyStats(ctx context.Context, userID string, date time.Time) (domain.DailyStats, error) {
	dateStr := date.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, image_ref, calories, protein, fat, carbs, caption, created_at
		 FROM food_logs
		 WHERE user_id = ? AND created_at >= ? AND created_at < ?
		 ORDER BY created_at`,
		userID, dateStr, dateStr+"T24",
	)
	if err != nil {
		return domain.DailyStats{}, fmt.Errorf("get daily stats: %w", err)
	}
	defer rows.Close()

	stats := domain.DailyStats{Date: dateStr}
	for rows.Next() {
		var m domain.FoodLog
		var createdAt string
		if err := rows.Scan(&m.ID, &m.Name, &m.ImageRef, &m.Calories, &m.Protein, &m.Fat, &m.Carbs, &m.Caption, &createdAt); err != nil {
			return domain.DailyStats{}, fmt.Errorf("scan food log: %w", err)
		}
		m.UserID = userID
		m.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if m.CreatedAt.IsZero() {
			m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}

		stats.TotalKcal += m.Calories
		stats.Protein += m.Protein
		stats.Fat += m.Fat
		stats.Carbs += m.Carbs
		stats.Meals = append(stats.Meals, m)
	}
	return stats, rows.Err()
}
