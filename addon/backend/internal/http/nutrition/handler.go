package nutrition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ognick/zabkiss/internal/domain"
	"github.com/ognick/zabkiss/pkg/visionllm"
)

type nutritionRepo interface {
	ListByDate(ctx context.Context, userID string, date time.Time) ([]domain.FoodLog, error)
	Update(ctx context.Context, log domain.FoodLog) error
	Delete(ctx context.Context, userID string, id int64) error
	GetTargets(ctx context.Context, userID string) (domain.NutritionTargets, error)
	SaveTargets(ctx context.Context, t domain.NutritionTargets) error
	RegenerateToken(ctx context.Context, userID string) (string, error)
	ListUsers(ctx context.Context) ([]string, error)
	ResolveUserByToken(ctx context.Context, token string) (string, error)
	GetWeeklyCache(ctx context.Context, userID, period string) (string, string, error)
	SaveWeeklyCache(ctx context.Context, userID, period, requestHash, report string) error
	InvalidateWeeklyCache(ctx context.Context, userID string) error
	MaxMealCreatedAt(ctx context.Context, userID string, start, end time.Time) (string, error)
}

type Handler struct {
	repo nutritionRepo
	llm  visionllm.Client
	log  logger
}

type logger interface {
	Info(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

func NewIngressHandler(repo nutritionRepo, llm visionllm.Client, log logger) *Handler {
	return &Handler{repo: repo, llm: llm, log: log}
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListUsers(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []string{}
	}
	writeJSON(w, users)
}

func (h *Handler) GetEntries(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	dateStr := r.URL.Query().Get("date")
	if userID == "" || dateStr == "" {
		http.Error(w, `{"error":"user_id and date required"}`, http.StatusBadRequest)
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, `{"error":"invalid date"}`, http.StatusBadRequest)
		return
	}

	entries, err := h.repo.ListByDate(r.Context(), userID, date)
	if err != nil {
		h.log.Error("get entries", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []domain.FoodLog{}
	}
	writeJSON(w, entries)
}

func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	var entry domain.FoodLog
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	entry.ID = id
	if entry.UserID == "" {
		http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
		return
	}
	if entry.Calories < 0 || entry.Protein < 0 || entry.Fat < 0 || entry.Carbs < 0 {
		http.Error(w, `{"error":"macros must be non-negative"}`, http.StatusBadRequest)
		return
	}

	if err := h.repo.Update(r.Context(), entry); err != nil {
		h.log.Error("update entry", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	h.repo.InvalidateWeeklyCache(r.Context(), entry.UserID)
	writeJSON(w, map[string]bool{"ok": true})
}

func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
		return
	}

	if err := h.repo.Delete(r.Context(), userID, id); err != nil {
		h.log.Error("delete entry", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	h.repo.InvalidateWeeklyCache(r.Context(), userID)
	writeJSON(w, map[string]bool{"ok": true})
}

func (h *Handler) GetTargets(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
		return
	}

	targets, err := h.repo.GetTargets(r.Context(), userID)
	if err != nil {
		h.log.Error("get targets", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	targets.APIToken = maskToken(targets.APIToken)
	writeJSON(w, targets)
}

func maskToken(tok string) string {
	if len(tok) <= 8 {
		return "****"
	}
	return tok[:4] + "****" + tok[len(tok)-4:]
}

func (h *Handler) SaveTargets(w http.ResponseWriter, r *http.Request) {
	var targets domain.NutritionTargets
	if err := json.NewDecoder(r.Body).Decode(&targets); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if targets.UserID == "" {
		http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
		return
	}
	if targets.Calories < 500 || targets.Calories > 15000 {
		http.Error(w, `{"error":"calories out of range (500-15000)"}`, http.StatusBadRequest)
		return
	}
	if targets.Protein < 0 || targets.Fat < 0 || targets.Carbs < 0 {
		http.Error(w, `{"error":"macros must be non-negative"}`, http.StatusBadRequest)
		return
	}
	if targets.HeightCm < 0 || targets.HeightCm > 300 {
		http.Error(w, `{"error":"height out of range"}`, http.StatusBadRequest)
		return
	}
	if targets.WeightKg < 0 || targets.WeightKg > 500 {
		http.Error(w, `{"error":"weight out of range"}`, http.StatusBadRequest)
		return
	}
	if targets.Sex != "" && targets.Sex != "male" && targets.Sex != "female" {
		http.Error(w, `{"error":"sex must be male or female"}`, http.StatusBadRequest)
		return
	}

	if err := h.repo.SaveTargets(r.Context(), targets); err != nil {
		h.log.Error("save targets", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	h.repo.InvalidateWeeklyCache(r.Context(), targets.UserID)
	writeJSON(w, map[string]bool{"ok": true})
}

func (h *Handler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	token, err := h.repo.RegenerateToken(r.Context(), userID)
	if err != nil {
		h.log.Error("regenerate token", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"token": token})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if rest, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return rest
	}
	return r.URL.Query().Get("token")
}

func (h *Handler) ExternalDaily(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, `{"error":"token required"}`, http.StatusUnauthorized)
		return
	}

	userID, err := h.repo.ResolveUserByToken(r.Context(), token)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	dateStr := r.URL.Query().Get("date")
	date := time.Now()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, `{"error":"invalid date"}`, http.StatusBadRequest)
			return
		}
		date = parsed
	}

	entries, err := h.repo.ListByDate(r.Context(), userID, date)
	if err != nil {
		h.log.Error("external daily", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []domain.FoodLog{}
	}

	targets, err := h.repo.GetTargets(r.Context(), userID)
	if err != nil {
		h.log.Error("get targets for daily", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	targetMacros := domain.Macros{
		Calories: targets.Calories,
		Protein:  targets.Protein,
		Fat:      targets.Fat,
		Carbs:    targets.Carbs,
	}

	var consumed domain.Macros
	for _, m := range entries {
		consumed.Calories += m.Calories
		consumed.Protein += m.Protein
		consumed.Fat += m.Fat
		consumed.Carbs += m.Carbs
	}

	remaining := domain.Macros{
		Calories: max(0, targetMacros.Calories-consumed.Calories),
		Protein:  max(0, targetMacros.Protein-consumed.Protein),
		Fat:      max(0, targetMacros.Fat-consumed.Fat),
		Carbs:    max(0, targetMacros.Carbs-consumed.Carbs),
	}

	report := domain.DailyReport{
		Date:           date.Format("2006-01-02"),
		Consumed:       consumed,
		Remaining:      remaining,
		Targets:        targetMacros,
		Meals:          entries,
		Recommendation: dailyRecommendation(targetMacros, consumed),
	}

	writeJSON(w, report)
}

func (h *Handler) ExternalWeekly(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	period := r.URL.Query().Get("period")
	if token == "" || period == "" {
		http.Error(w, `{"error":"token and period required"}`, http.StatusBadRequest)
		return
	}

	userID, err := h.repo.ResolveUserByToken(r.Context(), token)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	targets, err := h.repo.GetTargets(r.Context(), userID)
	if err != nil {
		h.log.Error("get targets for weekly", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	targetsJSON, _ := json.Marshal(targets)

	date, err := mondayOfISOWeek(period)
	if err != nil {
		http.Error(w, `{"error":"invalid period, use 2026-W23 format"}`, http.StatusBadRequest)
		return
	}

	monday, sunday := weekRange(date)
	lastMealTime, err := h.repo.MaxMealCreatedAt(r.Context(), userID, monday, sunday)
	if err != nil {
		h.log.Error("max meal time", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	requestHash := computeRequestHash(userID, period, string(targetsJSON), lastMealTime)

	cachedHash, cachedReport, err := h.repo.GetWeeklyCache(r.Context(), userID, period)
	if err == nil && cachedHash == requestHash && cachedReport != "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cachedReport))
		return
	}

	report, err := generateWeeklyReport(r.Context(), h.repo, userID, monday, sunday, targets)
	if err != nil {
		h.log.Error("generate weekly report", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if h.llm != nil {
		prompt := weeklyLLMPrompt(report, targets)
		rec, llmErr := h.llm.ChatCompletion(r.Context(), []visionllm.Message{
			{Role: "system", Content: []visionllm.ContentPart{{Type: "text", Text: "You are an expert nutritionist. Respond in Russian with 3-5 sentences."}}},
			{Role: "user", Content: []visionllm.ContentPart{{Type: "text", Text: prompt}}},
		})
		if llmErr != nil {
			h.log.Error("llm weekly recommendation", "err", llmErr)
		} else {
			report.Recommendation = rec
		}
	}

	reportJSON, _ := json.Marshal(report)
	h.repo.SaveWeeklyCache(r.Context(), userID, period, requestHash, string(reportJSON))

	writeJSON(w, report)
}

func mondayOfISOWeek(period string) (time.Time, error) {
	var year, week int
	if _, err := fmt.Sscanf(period, "%d-W%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.Local)
	if isoYear, _ := jan4.ISOWeek(); isoYear != year {
		jan4 = time.Date(year+1, 1, 4, 0, 0, 0, 0, time.Local)
	}
	for jan4.Weekday() != time.Monday {
		jan4 = jan4.AddDate(0, 0, -1)
	}
	_, jan4Week := jan4.ISOWeek()
	offset := (week - jan4Week) * 7
	return jan4.AddDate(0, 0, offset), nil
}
