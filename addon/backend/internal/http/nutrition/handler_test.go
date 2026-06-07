package nutrition

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ognick/zabkiss/internal/domain"
)

type mockNutritionRepo struct {
	entries     map[string][]domain.FoodLog
	targets     map[string]domain.NutritionTargets
	maxMealTime map[string]string
}

func newMockRepo() *mockNutritionRepo {
	return &mockNutritionRepo{
		entries:     make(map[string][]domain.FoodLog),
		targets:     make(map[string]domain.NutritionTargets),
		maxMealTime: make(map[string]string),
	}
}

func (m *mockNutritionRepo) ListByDate(ctx context.Context, userID string, date time.Time) ([]domain.FoodLog, error) {
	return m.entries[userID], nil
}

func (m *mockNutritionRepo) Update(ctx context.Context, log domain.FoodLog) error {
	for i, e := range m.entries[log.UserID] {
		if e.ID == log.ID {
			m.entries[log.UserID][i] = log
			return nil
		}
	}
	return nil
}

func (m *mockNutritionRepo) Delete(ctx context.Context, userID string, id int64) error {
	return nil
}

func (m *mockNutritionRepo) GetTargets(ctx context.Context, userID string) (domain.NutritionTargets, error) {
	if t, ok := m.targets[userID]; ok {
		return t, nil
	}
	return domain.NutritionTargets{UserID: userID, Calories: 2000, Protein: 60, Fat: 65, Carbs: 250, APIToken: "tok-" + userID}, nil
}

func (m *mockNutritionRepo) SaveTargets(ctx context.Context, t domain.NutritionTargets) error {
	m.targets[t.UserID] = t
	return nil
}

func (m *mockNutritionRepo) RegenerateToken(ctx context.Context, userID string) (string, error) {
	return "new-" + userID, nil
}

func (m *mockNutritionRepo) ListUsers(ctx context.Context) ([]string, error) {
	return []string{"u1", "u2"}, nil
}

func (m *mockNutritionRepo) ResolveUserByToken(ctx context.Context, token string) (string, error) {
	return "u1", nil
}

func (m *mockNutritionRepo) GetWeeklyCache(ctx context.Context, userID, period string) (string, string, error) {
	return "", "", nil
}

func (m *mockNutritionRepo) SaveWeeklyCache(ctx context.Context, userID, period, requestHash, report string) error {
	return nil
}

func (m *mockNutritionRepo) InvalidateWeeklyCache(ctx context.Context, userID string) error {
	return nil
}

func (m *mockNutritionRepo) MaxMealCreatedAt(ctx context.Context, userID string, start, end time.Time) (string, error) {
	return "2026-06-07 10:00:00", nil
}

func TestIngressListUsers(t *testing.T) {
	repo := newMockRepo()
	h := NewIngressHandler(repo, nil, nil)
	r := chi.NewRouter()
	r.Get("/nutrition/api/users", h.ListUsers)

	req := httptest.NewRequest("GET", "/nutrition/api/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	var users []string
	json.NewDecoder(w.Body).Decode(&users)
	if len(users) != 2 {
		t.Errorf("want 2 users, got %d", len(users))
	}
}

func TestIngressGetEntries(t *testing.T) {
	repo := newMockRepo()
	h := NewIngressHandler(repo, nil, nil)
	r := chi.NewRouter()
	r.Get("/nutrition/api/entries", h.GetEntries)

	req := httptest.NewRequest("GET", "/nutrition/api/entries?user_id=u1&date=2026-06-07", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIngressGetTargets(t *testing.T) {
	repo := newMockRepo()
	h := NewIngressHandler(repo, nil, nil)
	r := chi.NewRouter()
	r.Get("/nutrition/api/targets", h.GetTargets)

	req := httptest.NewRequest("GET", "/nutrition/api/targets?user_id=u1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var targets domain.NutritionTargets
	json.NewDecoder(w.Body).Decode(&targets)
	if targets.Calories != 2000 {
		t.Errorf("want 2000, got %d", targets.Calories)
	}
	if targets.APIToken != "" && len(targets.APIToken) > 8 {
		t.Errorf("api_token should be masked, got %s", targets.APIToken)
	}
}

func TestIngressSaveTargets(t *testing.T) {
	repo := newMockRepo()
	h := NewIngressHandler(repo, nil, nil)
	r := chi.NewRouter()
	r.Put("/nutrition/api/targets", h.SaveTargets)

	body := bytes.NewBufferString(`{"user_id":"u1","calories":2500,"protein":80,"fat":70,"carbs":280,"height_cm":180,"weight_kg":80,"sex":"male","body_fat_pct":15}`)
	req := httptest.NewRequest("PUT", "/nutrition/api/targets", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalDaily(t *testing.T) {
	repo := newMockRepo()
	repo.entries["u1"] = []domain.FoodLog{
		{ID: 1, UserID: "u1", Name: "Овсянка", Calories: 300, Protein: 10, Fat: 5, Carbs: 50, CreatedAt: time.Now()},
	}
	h := NewIngressHandler(repo, nil, nil)
	r := chi.NewRouter()
	r.Get("/api/zabkiss/nutrition/daily", h.ExternalDaily)

	req := httptest.NewRequest("GET", "/api/zabkiss/nutrition/daily?token=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var report domain.DailyReport
	json.NewDecoder(w.Body).Decode(&report)
	if report.Consumed.Calories != 300 {
		t.Errorf("want 300 consumed, got %d", report.Consumed.Calories)
	}
}
