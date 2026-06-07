package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
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
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS food_logs (
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
		);`,
		`CREATE INDEX IF NOT EXISTS idx_food_logs_user_date ON food_logs(user_id, created_at);`,
		`CREATE TABLE IF NOT EXISTS user_targets (
			user_id      TEXT PRIMARY KEY,
			calories     INTEGER NOT NULL DEFAULT 2000,
			protein      INTEGER NOT NULL DEFAULT 60,
			fat          INTEGER NOT NULL DEFAULT 65,
			carbs        INTEGER NOT NULL DEFAULT 250,
			height_cm    INTEGER NOT NULL DEFAULT 0,
			weight_kg    REAL    NOT NULL DEFAULT 0.0,
			sex          TEXT    NOT NULL DEFAULT '',
			body_fat_pct REAL    NOT NULL DEFAULT 0.0,
			api_token    TEXT    NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS weekly_cache (
			user_id      TEXT NOT NULL,
			period       TEXT NOT NULL,
			request_hash TEXT NOT NULL,
			report       TEXT NOT NULL,
			PRIMARY KEY (user_id, period)
		);`,
	}
	for _, m := range migrations {
		if _, err := r.db.Exec(m); err != nil {
			return err
		}
	}
	return nil
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

func (r *NutritionRepo) ListByDate(ctx context.Context, userID string, date time.Time) ([]domain.FoodLog, error) {
	dateStr := date.Format("2006-01-02")
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, image_ref, calories, protein, fat, carbs, caption, created_at
		 FROM food_logs
		 WHERE user_id = ? AND created_at >= ? AND created_at < ?
		 ORDER BY created_at`,
		userID, dateStr, dateStr+"T24",
	)
	if err != nil {
		return nil, fmt.Errorf("list food logs: %w", err)
	}
	defer rows.Close()

	var logs []domain.FoodLog
	for rows.Next() {
		var m domain.FoodLog
		var createdAt string
		if err := rows.Scan(&m.ID, &m.Name, &m.ImageRef, &m.Calories, &m.Protein, &m.Fat, &m.Carbs, &m.Caption, &createdAt); err != nil {
			return nil, fmt.Errorf("scan food log: %w", err)
		}
		m.UserID = userID
		m.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if m.CreatedAt.IsZero() {
			m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		logs = append(logs, m)
	}
	return logs, rows.Err()
}

func (r *NutritionRepo) Update(ctx context.Context, log domain.FoodLog) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE food_logs SET name=?, calories=?, protein=?, fat=?, carbs=? WHERE id=? AND user_id=?`,
		log.Name, log.Calories, log.Protein, log.Fat, log.Carbs, log.ID, log.UserID,
	)
	if err != nil {
		return fmt.Errorf("update food log: %w", err)
	}
	return nil
}

func (r *NutritionRepo) Delete(ctx context.Context, userID string, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM food_logs WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete food log: %w", err)
	}
	return nil
}

func (r *NutritionRepo) GetTargets(ctx context.Context, userID string) (domain.NutritionTargets, error) {
	token := generateToken()
	_, _ = r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO user_targets (user_id, api_token) VALUES (?, ?)`,
		userID, token,
	)
	var t domain.NutritionTargets
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, calories, protein, fat, carbs, height_cm, weight_kg, sex, body_fat_pct, api_token
		 FROM user_targets WHERE user_id=?`, userID,
	).Scan(&t.UserID, &t.Calories, &t.Protein, &t.Fat, &t.Carbs, &t.HeightCm, &t.WeightKg, &t.Sex, &t.BodyFatPct, &t.APIToken)
	if err != nil {
		return domain.NutritionTargets{}, fmt.Errorf("get user targets: %w", err)
	}
	return t, nil
}

func (r *NutritionRepo) SaveTargets(ctx context.Context, t domain.NutritionTargets) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_targets (user_id, calories, protein, fat, carbs, height_cm, weight_kg, sex, body_fat_pct)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   calories=excluded.calories, protein=excluded.protein, fat=excluded.fat, carbs=excluded.carbs,
		   height_cm=excluded.height_cm, weight_kg=excluded.weight_kg, sex=excluded.sex,
		   body_fat_pct=excluded.body_fat_pct`,
		t.UserID, t.Calories, t.Protein, t.Fat, t.Carbs, t.HeightCm, t.WeightKg, t.Sex, t.BodyFatPct,
	)
	if err != nil {
		return fmt.Errorf("save targets: %w", err)
	}
	return nil
}

func (r *NutritionRepo) RegenerateToken(ctx context.Context, userID string) (string, error) {
	token := generateToken()
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_targets SET api_token=? WHERE user_id=?`,
		token, userID,
	)
	if err != nil {
		return "", fmt.Errorf("regenerate token: %w", err)
	}
	return token, nil
}

func (r *NutritionRepo) ListUsers(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM food_logs ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *NutritionRepo) ResolveUserByToken(ctx context.Context, token string) (string, error) {
	var userID string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM user_targets WHERE api_token=?`, token,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("invalid token")
	}
	if err != nil {
		return "", fmt.Errorf("resolve token: %w", err)
	}
	return userID, nil
}

func (r *NutritionRepo) GetWeeklyCache(ctx context.Context, userID, period string) (string, string, error) {
	var requestHash, report string
	err := r.db.QueryRowContext(ctx,
		`SELECT request_hash, report FROM weekly_cache WHERE user_id=? AND period=?`,
		userID, period,
	).Scan(&requestHash, &report)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("get weekly cache: %w", err)
	}
	return requestHash, report, nil
}

func (r *NutritionRepo) SaveWeeklyCache(ctx context.Context, userID, period, requestHash, report string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO weekly_cache (user_id, period, request_hash, report)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, period) DO UPDATE SET request_hash=excluded.request_hash, report=excluded.report`,
		userID, period, requestHash, report,
	)
	if err != nil {
		return fmt.Errorf("save weekly cache: %w", err)
	}
	return nil
}

func (r *NutritionRepo) InvalidateWeeklyCache(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM weekly_cache WHERE user_id=?`, userID)
	return err
}

func (r *NutritionRepo) MaxMealCreatedAt(ctx context.Context, userID string, start, end time.Time) (string, error) {
	var maxTime sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(created_at) FROM food_logs WHERE user_id=? AND created_at >= ? AND created_at < ?`,
		userID, start.Format("2006-01-02"), end.Add(24*time.Hour).Format("2006-01-02"),
	).Scan(&maxTime)
	if err != nil {
		return "", fmt.Errorf("max meal created_at: %w", err)
	}
	if !maxTime.Valid {
		return "none", nil
	}
	return maxTime.String, nil
}

func generateToken() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}
