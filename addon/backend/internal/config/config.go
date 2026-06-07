package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr                  string
	DBPath                string
	LogLevel              string
	OpenCodeAPIKey        string
	OpenCodeBaseURL       string
	OpenCodeModel         string
	AllowedEmails         []string
	HAToken               string
	HAURL                 string
	PolicyCacheTTLSeconds int
	YouTubeAPIKey         string
	TelegramBotToken      string
	TelegramLLMModel      string
	TelegramAPIToken      string
	TelegramAllowedUsers  []string
}

func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		Addr:                  getEnv("ADDR", ":8080"),
		DBPath:                getEnv("DB_PATH", "/data/zabkiss.db"),
		LogLevel:              getEnv("LOG_LEVEL", "debug"),
		OpenCodeAPIKey:        getEnv("OPENCODE_API_KEY", ""),
		OpenCodeBaseURL:       getEnv("OPENCODE_BASE_URL", "https://opencode.ai/zen/go/v1"),
		OpenCodeModel:         getEnv("OPENCODE_MODEL", "deepseek-v4-flash"),
		AllowedEmails:         parseList(getEnv("ALLOWED_EMAILS", "")),
		HAToken:               getEnv("HA_TOKEN", ""),
		HAURL:                 getEnv("HA_URL", "http://homeassistant:8123"),
		PolicyCacheTTLSeconds: parseInt(getEnv("POLICY_CACHE_TTL_SECONDS", "10")),
		YouTubeAPIKey:         getEnv("YOUTUBE_API_KEY", ""),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramLLMModel:      getEnv("TELEGRAM_LLM_MODEL", "kimi-k2.6"),
		TelegramAPIToken:      getEnv("TELEGRAM_API_TOKEN", ""),
		TelegramAllowedUsers:  parseList(getEnv("TELEGRAM_ALLOWED_USER_IDS", "")),
	}
}

func parseList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func parseInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 60
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
