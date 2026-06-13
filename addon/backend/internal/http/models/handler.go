package models

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ognick/zabkiss/pkg/llmmodels"
	"github.com/ognick/zabkiss/pkg/logger"
)

// repoGateway — узкий интерфейс для получения списка моделей.
// Нужен, чтобы тесты могли подсунуть фейковый список без HTTP.
type repoGateway interface {
	List(ctx context.Context) ([]llmmodels.Enriched, error)
}

// Handler отдаёт список моделей OpenCode Zen с метриками скорости/интеллекта.
// Кэширует результат на cacheTTL, чтобы не дёргать API на каждый запрос UI.
type Handler struct {
	repo     repoGateway
	log      logger.Logger
	cacheTTL time.Duration

	mu       sync.Mutex
	cached   []llmmodels.Enriched
	cachedAt time.Time
}

func New(repo repoGateway, log logger.Logger) *Handler {
	return &Handler{repo: repo, log: log, cacheTTL: 60 * time.Second}
}

// List обрабатывает GET /api/zabkiss/models.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	if h.cached != nil && time.Since(h.cachedAt) < h.cacheTTL {
		cached := h.cached
		h.mu.Unlock()
		h.write(w, cached)
		return
	}
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	models, err := h.repo.List(ctx)
	if err != nil {
		h.log.Error("list models failed", "err", err)
		http.Error(w, `{"error":"failed to list models"}`, http.StatusBadGateway)
		return
	}

	h.mu.Lock()
	h.cached = models
	h.cachedAt = time.Now()
	h.mu.Unlock()
	h.write(w, models)
}

func (h *Handler) write(w http.ResponseWriter, models []llmmodels.Enriched) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"models": models,
	}); err != nil {
		h.log.Warn("models response write failed", "err", err)
	}
}
