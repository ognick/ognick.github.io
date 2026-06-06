# Telegram Nutrition Bot — Design Spec

## Overview

Add a Telegram bot webhook to the ZabKiss add-on that allows users to send food photos with descriptions, receive AI-powered calorie/BJU analysis with daily stats and recommendations, and expose a JSON API endpoint for daily nutrition statistics.

## Architecture

Follow the existing repository pattern: domain types → service with gate interfaces → SQLite repository. New packages are additive and do not modify existing code.

```
internal/http/telegram/       # webhook handler
internal/service/nutrition.go # NutritionService (orchestrator)
internal/domain/nutrition.go  # FoodLog, DailyStats, FoodAnalysis
internal/repository/sqlite/
  nutrition_repo.go           # CRUD + daily aggregation
pkg/gemini/                   # Gemini API client (vision)
pkg/telegram/                 # Telegram Bot API client (sendMessage, getFile)
```

## Data Flow

```
User sends photo + caption via Telegram
  → POST /telegram/webhook (chi router)
  → Parse Update (chat_id, photo[], caption, from.id)
  → telegram.Client.DownloadFile(file_id) → []byte
  → NutritionService.AnalyzeFood(userID, imageBytes, caption)
      1. gemini.Client.AnalyzeFood(image, mimeType, caption) → FoodAnalysis
      2. nutritionRepo.Save(userID, FoodLog)
      3. nutritionRepo.GetDailyStats(userID, today) → DailyStats
      4. Format reply: meal BJU + daily totals + recommendation
  → telegram.Client.SendMessage(chat_id, text)

External API:
  GET /api/zabkiss/nutrition/daily?user_id=X&date=YYYY-MM-DD
  Authorization: Bearer <telegram_api_token>
  → nutritionRepo.GetDailyStats(userID, date) → JSON
```

## Domain Types

All nutritional values are `int` (whole numbers).

```go
type FoodLog struct {
    ID        int64
    UserID    string // Telegram user ID
    Name      string // meal name
    ImageRef  string // Telegram file_id
    Calories  int
    Protein   int // grams
    Fat       int
    Carbs     int
    Caption   string // user's text description
    CreatedAt time.Time
}

type DailyStats struct {
    Date      string
    TotalKcal int
    Protein   int
    Fat       int
    Carbs     int
    Meals     []FoodLog
}

type FoodAnalysis struct {
    Name           string
    Calories       int
    Protein        int
    Fat            int
    Carbs          int
    Recommendation string
}
```

## Database

Separate SQLite file at `/data/nutrition.db` (hardcoded path).

```sql
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
CREATE INDEX idx_food_logs_user_date ON food_logs(user_id, created_at);
```

## Gemini Client (`pkg/gemini/`)

API endpoint: `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={api_key}`

Request format: Gemini multimodal `contents[].parts[]` — text part + inline base64 image part.

Response: parsed into `FoodAnalysis` via JSON struct.

System prompt (as provided):

```
You are an expert nutritionist and food analyst.
Analyze the food image and estimate nutritional information.
[... full prompt as specified ...]
Schema:
{
  "meal_name": "...",
  "overall_confidence": 0,
  "items": [{ "name": "...", "estimated_weight_g": 0, "calories": 0, "protein_g": 0, "fat_g": 0, "carbs_g": 0, "confidence": 0, "notes": "" }],
  "totals": { "calories": 0, "protein_g": 0, "fat_g": 0, "carbs_g": 0 },
  "analysis": { "portion_size": "...", "notes": "" },
  "recommendation": "краткий диетологический совет, 1 предложение"
}
```

Mapping to `FoodAnalysis`:
- `meal_name` → `Name`
- `totals.calories/protein_g/fat_g/carbs_g` → `Calories/Protein/Fat/Carbs`
- `recommendation` → `Recommendation`

## Telegram Bot Client (`pkg/telegram/`)

Minimal client for two operations:
- `DownloadFile(fileID string) ([]byte, error)` — fetches photo via `getFile` + download
- `SendMessage(chatID int64, text string) error` — sends text reply

Uses `telegram_bot_token` from config. Base URL: `https://api.telegram.org/bot{token}/`.

## Nutrition Service (`internal/service/nutrition.go`)

Interface-based gates for DI:

```go
type geminiGateway interface {
    AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error)
}

type telegramGateway interface {
    DownloadFile(ctx context.Context, fileID string) ([]byte, error)
    SendMessage(ctx context.Context, chatID int64, text string) error
}

type nutritionRepo interface {
    Save(ctx context.Context, log domain.FoodLog) error
    GetDailyStats(ctx context.Context, userID string, date time.Time) (domain.DailyStats, error)
}
```

Public methods:

`AnalyzeFood(ctx, userID, fileID, caption string) (string, error)`:
1. Download photo bytes via `telegramGateway`
2. Call `geminiGateway.AnalyzeFood`
3. Save `FoodLog` via `nutritionRepo`
4. Get daily stats via `nutritionRepo`
5. Format reply text: "🍽 {name}: {cal} ккал, Б: {p}г, Ж: {f}г, У: {c}г\n📊 За сегодня: ...\n💡 {recommendation}"
6. Return text

`GetDailyStats(ctx, userID string, date time.Time) (domain.DailyStats, error)`:
- Delegates to `nutritionRepo.GetDailyStats` — used by stats API endpoint

## Telegram Webhook Handler (`internal/http/telegram/`)

- `POST /telegram/webhook`
- Parse Telegram `Update` JSON
- Extract: `message.chat.id`, `message.from.id` (as userID), `message.photo[]` (last = highest res), `message.caption`
- If no photo → reply "Отправьте фото еды с описанием"
- Call `nutritionService.AnalyzeFood(userID, fileID, caption)`
- Call `telegramGateway.SendMessage(chatID, text)` with result
- Always respond 200 to Telegram immediately (Telegram retries on non-200)

Error handling:
- Download failure → "Не удалось загрузить фото"
- Gemini failure → "Не удалось проанализировать фото"
- If `meal_name == "NOT_FOOD"` → "На фото не похоже на еду"

## Stats API Endpoint

```
GET /api/zabkiss/nutrition/daily?user_id=<telegram_id>&date=YYYY-MM-DD
Authorization: Bearer <telegram_api_token>
```

- Registered as chi route in main.go
- Auth middleware: check `Authorization: Bearer <telegram_api_token>`
- `date` optional, defaults to today
- Returns JSON `DailyStats` with `meals` array
- 401 if token invalid, 400 if `user_id` missing

## Config (addon/config.yaml + env vars)

| Option key | Env var | Default | Required |
|---|---|---|---|
| `telegram_bot_token` | `TELEGRAM_BOT_TOKEN` | `""` | Yes (for bot) |
| `telegram_llm_api_key` | `TELEGRAM_LLM_API_KEY` | `""` | Yes |
| `telegram_llm_model` | `TELEGRAM_LLM_MODEL` | `gemini-2.5-pro` | No |
| `telegram_api_token` | `TELEGRAM_API_TOKEN` | `""` | No (auto-generated UUID if empty) |

`telegram_api_token` generation: on startup, if env value is empty, generate a UUID, log it prominently, and store it in `nutrition.db` metadata table for persistence across restarts.

## Testing

- `internal/service/nutrition_test.go` — unit tests with mocked gates
- `internal/repository/sqlite/nutrition_repo_test.go` — SQLite integration tests
- `internal/http/telegram/handler_test.go` — webhook tests with httptest
- `pkg/gemini/client_test.go` — unit tests with mocked HTTP transport
- `pkg/telegram/client_test.go` — unit tests with mocked HTTP transport

## Out of Scope

- Auto-registration of Telegram webhook (user configures manually)
- Multi-language support (Russian only initially)
- User-specific daily calorie targets
- Photo history browsing beyond the daily stats API
