package llmmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ognick/zabkiss/pkg/logger"
)

// liveModel — элемент из ответа GET /v1/models.
type liveModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// liveResponse — обёртка над {object, data:[...]}.
type liveResponse struct {
	Object string      `json:"object"`
	Data   []liveModel `json:"data"`
}

// Enriched — модель, отдаваемая наружу: live ID + каталожные метрики.
type Enriched struct {
	ID          string `json:"id"`
	Speed       int    `json:"speed"`
	Intelligence int   `json:"intelligence"`
	Family      string `json:"family"`
	Description string `json:"description"`
}

// Client забирает список моделей с OpenCode Zen /v1/models и обогащает
// метаданными из статического каталога.
type Client struct {
	baseURL string
	apiKey  string
	log     logger.Logger
	http    *http.Client
}

func NewClient(baseURL, apiKey string, log logger.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		log:     log,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// List возвращает обогащённый список моделей. Если запрос к API падает,
// возвращается список из статического каталога + ошибка.
func (c *Client) List(ctx context.Context) ([]Enriched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return c.fallback(), fmt.Errorf("build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Warn("opencode /models failed, using static catalog", "err", err)
		return c.fallback(), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.log.Warn("opencode /models non-200, using static catalog", "status", resp.StatusCode)
		return c.fallback(), nil
	}

	var live liveResponse
	if err := json.NewDecoder(resp.Body).Decode(&live); err != nil {
		c.log.Warn("opencode /models decode failed, using static catalog", "err", err)
		return c.fallback(), nil
	}

	out := make([]Enriched, 0, len(live.Data))
	for _, m := range live.Data {
		meta, ok := Catalog[m.ID]
		if !ok {
			meta = DefaultMeta(m.ID)
		}
		out = append(out, Enriched{
			ID:          m.ID,
			Speed:       meta.Speed,
			Intelligence: meta.Intelligence,
			Family:      meta.Family,
			Description: meta.Description,
		})
	}
	return out, nil
}

// fallback — список моделей из статического каталога, отсортированный по ID.
func (c *Client) fallback() []Enriched {
	out := make([]Enriched, 0, len(Catalog))
	for id, meta := range Catalog {
		out = append(out, Enriched{
			ID:          id,
			Speed:       meta.Speed,
			Intelligence: meta.Intelligence,
			Family:      meta.Family,
			Description: meta.Description,
		})
	}
	return out
}
