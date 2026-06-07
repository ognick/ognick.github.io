package sqlite

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ognick/zabkiss/internal/domain"
)

func newTestNutritionRepo(t *testing.T) (*NutritionRepo, func()) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	db.SetMaxOpenConns(1)
	r := &NutritionRepo{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	return r, func() { db.Close() }
}

func TestNutritionRepo_SaveAndListByDate(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	now := time.Now()
	log1 := domain.FoodLog{UserID: "u1", Name: "Овсянка", Calories: 300, Protein: 10, Fat: 5, Carbs: 50, CreatedAt: now}
	log2 := domain.FoodLog{UserID: "u1", Name: "Курица", Calories: 450, Protein: 40, Fat: 8, Carbs: 52, CreatedAt: now}

	if err := repo.Save(ctx, log1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, log2); err != nil {
		t.Fatal(err)
	}

	logs, err := repo.ListByDate(ctx, "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("want 2 logs, got %d", len(logs))
	}
	if logs[0].Name != "Овсянка" {
		t.Errorf("want Овсянка, got %s", logs[0].Name)
	}
}

func TestNutritionRepo_Update(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	log := domain.FoodLog{UserID: "u1", Name: "Овсянка", Calories: 300, Protein: 10, Fat: 5, Carbs: 50, CreatedAt: time.Now()}
	if err := repo.Save(ctx, log); err != nil {
		t.Fatal(err)
	}

	logs, _ := repo.ListByDate(ctx, "u1", time.Now())
	saved := logs[0]

	saved.Name = "Гречка"
	saved.Calories = 350
	if err := repo.Update(ctx, saved); err != nil {
		t.Fatal(err)
	}

	logs, _ = repo.ListByDate(ctx, "u1", time.Now())
	if logs[0].Name != "Гречка" || logs[0].Calories != 350 {
		t.Errorf("update failed: got %s/%d", logs[0].Name, logs[0].Calories)
	}
}

func TestNutritionRepo_Delete(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	log := domain.FoodLog{UserID: "u1", Name: "Овсянка", Calories: 300, Protein: 10, Fat: 5, Carbs: 50, CreatedAt: time.Now()}
	if err := repo.Save(ctx, log); err != nil {
		t.Fatal(err)
	}

	logs, _ := repo.ListByDate(ctx, "u1", time.Now())
	if err := repo.Delete(ctx, "u1", logs[0].ID); err != nil {
		t.Fatal(err)
	}

	logs, _ = repo.ListByDate(ctx, "u1", time.Now())
	if len(logs) != 0 {
		t.Errorf("want 0 logs after delete, got %d", len(logs))
	}
}

func TestNutritionRepo_TargetsCRUD(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	targets, err := repo.GetTargets(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if targets.Calories != 2000 {
		t.Errorf("default calories: want 2000, got %d", targets.Calories)
	}
	if targets.APIToken == "" {
		t.Error("api_token should be auto-generated")
	}

	targets.Calories = 2500
	targets.HeightCm = 180
	targets.WeightKg = 80.5
	targets.Sex = "male"
	targets.BodyFatPct = 15.0
	if err := repo.SaveTargets(ctx, targets); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetTargets(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Calories != 2500 || got.HeightCm != 180 || got.WeightKg != 80.5 || got.Sex != "male" || got.BodyFatPct != 15.0 {
		t.Errorf("save did not persist: %+v", got)
	}
}

func TestNutritionRepo_RegenerateToken(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	targets, _ := repo.GetTargets(ctx, "u1")
	oldToken := targets.APIToken

	newToken, err := repo.RegenerateToken(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if newToken == "" || newToken == oldToken {
		t.Error("token should be regenerated and different")
	}

	targets, _ = repo.GetTargets(ctx, "u1")
	if targets.APIToken != newToken {
		t.Errorf("token mismatch after regenerate")
	}
}

func TestNutritionRepo_ResolveToken(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	targets, _ := repo.GetTargets(ctx, "u1")
	token := targets.APIToken

	userID, err := repo.ResolveUserByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "u1" {
		t.Errorf("want u1, got %s", userID)
	}

	_, err = repo.ResolveUserByToken(ctx, "bad-token")
	if err == nil {
		t.Error("bad token should fail")
	}
}

func TestNutritionRepo_WeeklyCache(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	hash, report, err := repo.GetWeeklyCache(ctx, "u1", "2026-W23")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" || report != "" {
		t.Error("empty cache should return empty strings")
	}

	if err := repo.SaveWeeklyCache(ctx, "u1", "2026-W23", "abc123", `{"test":true}`); err != nil {
		t.Fatal(err)
	}

	hash, report, err = repo.GetWeeklyCache(ctx, "u1", "2026-W23")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "abc123" || report != `{"test":true}` {
		t.Errorf("cache mismatch: %s / %s", hash, report)
	}

	if err := repo.InvalidateWeeklyCache(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	hash, _, _ = repo.GetWeeklyCache(ctx, "u1", "2026-W23")
	if hash != "" {
		t.Error("cache should be gone after invalidate")
	}
}

func TestNutritionRepo_ListUsers(t *testing.T) {
	repo, cleanup := newTestNutritionRepo(t)
	defer cleanup()
	ctx := t.Context()

	repo.Save(ctx, domain.FoodLog{UserID: "u1", Name: "A", CreatedAt: time.Now()})
	repo.Save(ctx, domain.FoodLog{UserID: "u2", Name: "B", CreatedAt: time.Now()})
	repo.Save(ctx, domain.FoodLog{UserID: "u1", Name: "C", CreatedAt: time.Now()})

	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Errorf("want 2 distinct users, got %d: %v", len(users), users)
	}
}
