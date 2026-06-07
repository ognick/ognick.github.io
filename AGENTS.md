3# AGENTS.md

ZabKiss — LLM-powered natural language smart home control for Home Assistant via Yandex Alice.
Two components: HA Custom Integration (Python) and HA Custom Add-on (Go).
Also: Telegram nutrition bot for food photo analysis (calories/BJU) via Gemini Vision.

## Commands

```bash
# Go (from addon/backend/)
go run ./cmd/zabkiss                  # run locally (needs .env)
go test ./...                         # all tests (excludes build-tagged integration tests)
go test -v -tags integration -timeout 10m ./testing/   # container-based acceptance tests
go build -o bin/zabkiss ./cmd/zabkiss

# Python integration (from repo root)
ruff check custom_components/zabkiss/
mypy custom_components/zabkiss/
pytest tests/

# Docker (from addon/)
docker build -t zabkiss-addon:dev .
```

## Architecture (corrected)

The add-on is a **chi** router (not Gin — `.github/workflows/ci.yml:26` builds `./cmd/zabkiss`).

```
Alice POST /alice/webhook
  → Yandex OAuth token → ResolveUser() (cached in memory repo, checked against allowed_emails)
  → SmartHomeService.Process()
      1. policy.HAClient.GetEntities() → GET /api/zabkiss/policy (cached, TTL from POLICY_CACHE_TTL_SECONDS)
      2. ha.Client.GetDeviceInfos() → GET /api/states + /api/services (builds domain.Device with services+params)
      3. YouTube virtual service injected for media_player.* if YOUTUBE_API_KEY is set
      4. llm.Client.Execute() → OpenAI chat/completions with json_object format; prompt built from devices+memory
      5. ha.Client.CallService() → POST /api/services/{domain}/{service}
      6. Memory facts saved/forgotten via SQLite
  → Session inbox: if Alice deadline expires, result deferred to next request
```

No explicit `policy.Validate()` step — security boundary is the LLM system prompt (only whitelisted entity_ids + discovered services + parameter types/ranges are exposed).

## Config (runtime via /data/options.json, mapped to env vars by run.sh)

| Env var | Option key | Notes |
|---------|-----------|-------|
| `OPENCODE_API_KEY` | `opencode_api_key` | Required |
| `OPENCODE_BASE_URL` | `opencode_base_url` | Default: `https://opencode.ai/zen/go/v1` |
| `OPENCODE_MODEL` | `opencode_model` | Default: `deepseek-v4-flash`; dropdown for Alice smart home |
| `HA_TOKEN` | `ha_token` | Required. Long-lived HA token |
| `HA_URL` | `ha_url` | Default: `http://homeassistant:8123` |
| `ALLOWED_EMAILS` | `allowed_emails` | Comma-separated; users outside this list get `errForbidden` |
| `POLICY_CACHE_TTL_SECONDS` | `policy_cache_ttl_seconds` | Default: 60 (run.sh), 10 in config.yaml |
| `YOUTUBE_API_KEY` | `youtube_api_key` | Optional; enables YouTube search via `media_player.play_youtube` |
| `LOG_LEVEL` | `log_level` | `debug\|info\|warn\|error` |
| `DB_PATH` | — | Default: `/data/zabkiss.db` (SQLite) |
| `TELEGRAM_BOT_TOKEN` | `telegram_bot_token` | Optional; enables Telegram nutrition bot |
| `TELEGRAM_LLM_MODEL` | `telegram_llm_model` | Default: `kimi-k2.6`; dropdown for food analysis |
| `TELEGRAM_API_TOKEN` | `telegram_api_token` | Optional; Bearer token for nutrition stats API. Auto-generated UUID if empty when telegram enabled. |

## Integration (Python)

REST API: `GET/POST /api/zabkiss/policy` (requires_auth=True). Payload is `{"entities": ["light.kitchen", ...]}` — a flat list of entity_ids (not the full policy schema from CLAUDE.md). Storage uses HA's `Store` helper.

The frontend panel is `zabkiss-panel-v2.js` served from `custom_components/zabkiss/frontend/`. Must be committed.

## Repository pattern

- `pkg/` — domain-independent wrappers/libraries only. No business logic, no prompts, no domain types.
- `internal/domain/` — `Device`, `CommandResult`, `Action`, `ChatMessage`, `MemoryFact`, `User`, `FoodLog`, `DailyStats`, `FoodAnalysis`
- `internal/service/` — `SmartHomeService` orchestrates HA+LLM+memory; `NutritionService` handles food photo analysis via Telegram+LLM. Both use gate interfaces for DI.
- `internal/repository/repository.go` — `UserRepo`, `MemoryRepo` interfaces
- `internal/repository/memory/` — in-memory user token cache
- `internal/repository/sqlite/` — SQLite-backed memory facts + `nutrition_repo.go` (separate `/data/nutrition.db`)
- `internal/http/telegram/` — Telegram webhook handler (`POST /telegram/webhook`)
- `pkg/visionllm/` — generic vision LLM client (no domain logic, no prompts)
- `pkg/telegram/` — Telegram Bot API client (downloadFile, sendMessage)
- `pkg/sqlitedb/` — SQLite connection helper (no ORM, uses `modernc.org/sqlite`)
- All domain knowledge (prompts, parsing, business logic) lives in `internal/`
- Lifecycle via `ognick/goscade`: components register with `goscade.Register(lc, ...)` for ordered startup/shutdown

## Testing conventions

- Unit/integration tests: `go test ./...` from `addon/backend/`
- E2E/penetration tests: `addon/backend/testing/` (package `e2e`), uses `httptest.NewServer` with mocked Yandex OAuth
- Container acceptance test: `//go:build integration` tag, spins up real HA via testcontainers, run with `go test -v -tags integration -timeout 10m ./testing/`
- Test helpers in `testing/setup_test.go`: `newServer()`, `newYandexMock()`, `authedReq()`, `pingReq()`
- Service layer tests in `internal/service/smarthome_test.go`

## Key quirks

- `run.sh` maps HA Supervisor options.json to env vars; locally a `.env` file is loaded via `godotenv`
- The LLM prompt is printed to stderr on every request (`fmt.Fprintf(os.Stderr, "=== LLM PROMPT ===" ...)`)
- `number` domain entities get entity-specific min/max injected into `number.set_value` params from state attributes (not from services API)
- Unavailable/unknown devices get no services exposed → LLM tells user device is unavailable
- `commands` field in Alice request is used (not `original_utterance`) for LLM input; `original_utterance` is used for ping detection
- Session history stored in-memory (not persisted), grouped by request time with nanosecond disambiguation
