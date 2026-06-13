package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ognick/zabkiss/pkg/llmmodels"
)

type fakeRepo struct {
	models []llmmodels.Enriched
	err    error
	calls  int
}

func (f *fakeRepo) List(_ context.Context) ([]llmmodels.Enriched, error) {
	f.calls++
	return f.models, f.err
}

type nilLogger struct{}

func (nilLogger) Info(_ string, _ ...any)    {}
func (nilLogger) Debug(_ string, _ ...any)   {}
func (nilLogger) Warn(_ string, _ ...any)    {}
func (nilLogger) Error(_ string, _ ...any)   {}
func (nilLogger) Infof(_ string, _ ...any)   {}
func (nilLogger) Errorf(_ string, _ ...any)  {}

func TestList_ReturnsModels(t *testing.T) {
	repo := &fakeRepo{models: []llmmodels.Enriched{
		{ID: "deepseek-v4-flash", Speed: 5, Intelligence: 4, Family: "deepseek", Description: "fast"},
		{ID: "kimi-k2.6", Speed: 3, Intelligence: 4, Family: "kimi", Description: "smart"},
	}}
	h := New(repo, nilLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/zabkiss/models", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var resp struct {
		Models []llmmodels.Enriched `json:"models"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(resp.Models))
	}
	if resp.Models[0].ID != "deepseek-v4-flash" {
		t.Errorf("first model ID: got %q", resp.Models[0].ID)
	}
	if resp.Models[0].Speed != 5 || resp.Models[0].Intelligence != 4 {
		t.Errorf("first model meta: got speed=%d intelligence=%d", resp.Models[0].Speed, resp.Models[0].Intelligence)
	}
}

func TestList_CachesResult(t *testing.T) {
	repo := &fakeRepo{models: []llmmodels.Enriched{{ID: "a"}}}
	h := New(repo, nilLogger{})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/zabkiss/models", nil)
		w := httptest.NewRecorder()
		h.List(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: status %d", i, w.Code)
		}
	}
	if repo.calls != 1 {
		t.Errorf("repo.List called %d times, want 1 (cached)", repo.calls)
	}
}

func TestList_RepoErrorReturnsBadGateway(t *testing.T) {
	repo := &fakeRepo{err: context.DeadlineExceeded}
	h := New(repo, nilLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/zabkiss/models", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status: got %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "failed to list models") {
		t.Errorf("body should mention failure, got: %s", w.Body.String())
	}
}

func TestList_ContentTypeIsJSON(t *testing.T) {
	repo := &fakeRepo{models: []llmmodels.Enriched{{ID: "x"}}}
	h := New(repo, nilLogger{})

	req := httptest.NewRequest(http.MethodGet, "/api/zabkiss/models", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: got %q, want application/json...", ct)
	}
}

// Убедимся, что TTL кэша не вечный — после явной инвалидации cacheAt
// (которую мы имитируем через создание нового Handler) идёт свежий вызов.
func TestList_RefreshesAfterTTL(t *testing.T) {
	repo := &fakeRepo{models: []llmmodels.Enriched{{ID: "a"}}}
	h := New(repo, nilLogger{})
	h.cacheTTL = 1 * time.Millisecond

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/zabkiss/models", nil)
		w := httptest.NewRecorder()
		h.List(w, req)
		time.Sleep(2 * time.Millisecond)
	}
	if repo.calls < 2 {
		t.Errorf("expected at least 2 calls after TTL expiry, got %d", repo.calls)
	}
}
