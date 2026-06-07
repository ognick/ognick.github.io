package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ognick/goscade/v2"
	"github.com/ognick/zabkiss/internal/config"
	"github.com/ognick/zabkiss/internal/ha"
	"github.com/ognick/zabkiss/internal/http/alice"
	tghandler "github.com/ognick/zabkiss/internal/http/telegram"
	"github.com/ognick/zabkiss/internal/llm"
	"github.com/ognick/zabkiss/internal/policy"
	memoryrepo "github.com/ognick/zabkiss/internal/repository/memory"
	sqliterepo "github.com/ognick/zabkiss/internal/repository/sqlite"
	"github.com/ognick/zabkiss/internal/service"
	"github.com/ognick/zabkiss/pkg/httpserver"
	"github.com/ognick/zabkiss/pkg/logger"
	"github.com/ognick/zabkiss/pkg/sqlitedb"
	tgclient "github.com/ognick/zabkiss/pkg/telegram"
	"github.com/ognick/zabkiss/pkg/visionllm"
	"github.com/ognick/zabkiss/pkg/youtube"
)

// version задаётся при сборке через -ldflags "-X main.version=x.y.z".
var version = "dev"

func main() {
	startedAt := time.Now().Format(time.RFC3339)

	cfg := config.Load()

	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	slogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	log := logger.With(logger.NewSlogAdapter(slogger), "version", version, "started_at", startedAt)

	db, err := sqlitedb.New(cfg.DBPath)
	if err != nil {
		log.Error("db", "err", err)
		return
	}

	// Токены хранятся в памяти (временные данные, не переживают рестарт)
	userRepo := memoryrepo.NewUserRepo()

	// Личная память пользователей хранится в SQLite (персистентно)
	memoryRepo, err := sqliterepo.NewMemoryRepo(db.DB)
	if err != nil {
		log.Error("memory repo", "err", err)
		return
	}

	r := chi.NewRouter()
	r.Use(httpserver.RecoveryMiddleware(log))
	r.Use(middleware.Logger)
	if level == slog.LevelDebug {
		r.Use(httpserver.DebugMiddleware())
	}

	policyClient := policy.NewClient(
		cfg.HAURL,
		cfg.HAToken,
		time.Duration(cfg.PolicyCacheTTLSeconds)*time.Second,
		log,
	)
	haClient := ha.NewClient(cfg.HAURL, cfg.HAToken)
	llmClient := llm.NewClient(cfg.OpenCodeBaseURL, cfg.OpenCodeAPIKey, cfg.OpenCodeModel, log)

	var ytClient service.YouTubeGateway
	if cfg.YouTubeAPIKey != "" {
		ytClient = youtube.NewClient(cfg.YouTubeAPIKey)
		log.Info("youtube integration enabled")
	}

	svc := service.New(haClient, llmClient, policyClient, memoryRepo, ytClient, log)

	alice.New(svc, alice.NewAuth(userRepo, cfg.AllowedEmails), log).Register(r)

	// ── Telegram ────────────────────────────────────────────────────────────
	var tgHandler *tghandler.Handler
	if cfg.TelegramBotToken != "" && (cfg.TelegramLLMAPIKey != "" || cfg.OpenCodeAPIKey != "") {
		nutritionRepo, err := sqliterepo.NewNutritionRepo()
		if err != nil {
			log.Error("nutrition repo", "err", err)
		} else {
			apiKey := cfg.TelegramLLMAPIKey
			if apiKey == "" {
				apiKey = cfg.OpenCodeAPIKey
			}
			baseURL := cfg.TelegramLLMBaseURL
			if baseURL == "" {
				baseURL = cfg.OpenCodeBaseURL
			}
			visionClient := visionllm.NewClient(baseURL, apiKey, cfg.TelegramLLMModel)
			tgClient := tgclient.NewClient(cfg.TelegramBotToken)
			nutritionSvc := service.NewNutritionService(visionClient, tgClient, nutritionRepo, log)

			tgHandler = tghandler.NewHandler(nutritionSvc, tgClient, cfg.TelegramAllowedUsers, log)

			apiToken := cfg.TelegramAPIToken
			if apiToken == "" {
				apiToken = generateToken()
				log.Warn("TELEGRAM_API_TOKEN not set, generated", "token", apiToken)
			}

			r.Get("/api/zabkiss/nutrition/daily", func(w http.ResponseWriter, req *http.Request) {
				auth := req.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != apiToken {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				userID := req.URL.Query().Get("user_id")
				if userID == "" {
					http.Error(w, `{"error":"user_id required"}`, http.StatusBadRequest)
					return
				}
				dateStr := req.URL.Query().Get("date")
				date := time.Now()
				if dateStr != "" {
					parsed, err := time.Parse("2006-01-02", dateStr)
					if err != nil {
						http.Error(w, `{"error":"invalid date format, use YYYY-MM-DD"}`, http.StatusBadRequest)
						return
					}
					date = parsed
				}
				stats, err := nutritionSvc.GetDailyStats(req.Context(), userID, date)
				if err != nil {
					log.Error("get daily stats", "err", err)
					http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				encodeJSON(w, stats)
			})

			log.Info("telegram nutrition bot enabled")
		}
	}

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	host := localHost()
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		log.Info(method + " http://" + host + cfg.Addr + route)
		return nil
	}); err != nil {
		log.Error("failed to walk routes", "err", err)
	}

	lc := goscade.NewLifecycle(log, goscade.WithShutdownHook())
	goscade.Register(lc, db)
	goscade.Register(lc, policyClient)
	if tgHandler != nil {
		goscade.Register(lc, tgHandler)
	}
	goscade.Register(lc, httpserver.New(cfg.Addr, r), db, policyClient)

	if err := goscade.Run(context.Background(), lc, func() {
		log.Info("ZabKiss ready", "addr", cfg.Addr)
	}); err != nil {
		log.Error("fatal", "err", err)
	}
}

func localHost() string {
	name, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	if strings.HasSuffix(name, ".local") {
		return name
	}
	return name + ".local"
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

func encodeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v) //nolint:errcheck
}
