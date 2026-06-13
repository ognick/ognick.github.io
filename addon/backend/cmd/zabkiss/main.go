package main

import (
	"context"
	"fmt"
	"log/slog"
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
	modelshttp "github.com/ognick/zabkiss/internal/http/models"
	"github.com/ognick/zabkiss/internal/llm"
	"github.com/ognick/zabkiss/internal/policy"
	memoryrepo "github.com/ognick/zabkiss/internal/repository/memory"
	sqliterepo "github.com/ognick/zabkiss/internal/repository/sqlite"
	"github.com/ognick/zabkiss/internal/service"
	"github.com/ognick/zabkiss/pkg/httpserver"
	"github.com/ognick/zabkiss/pkg/llmmodels"
	"github.com/ognick/zabkiss/pkg/logger"
	"github.com/ognick/zabkiss/pkg/sqlitedb"
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

	modelsClient := llmmodels.NewClient(cfg.OpenCodeBaseURL, cfg.OpenCodeAPIKey, log)
	modelsHandler := modelshttp.New(modelsClient, log)
	r.With(ingressAuth).Get("/api/zabkiss/models", modelsHandler.List)

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

// ingressAuth ограничивает доступ к admin-эндпоинтам в режиме HA Supervisor.
// HA-фронтенд шлёт X-HA-Access с токеном SUPERVISOR_TOKEN; вне Supervisor'а
// (например, локальный dev) middleware пропускает все запросы.
func ingressAuth(next http.Handler) http.Handler {
	supervisorToken := os.Getenv("SUPERVISOR_TOKEN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if supervisorToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-HA-Access") != supervisorToken {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

