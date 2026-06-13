package config

import (
	"fmt"
	"net/url"
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
}

func Load() *Config {
	_ = godotenv.Load()
	cfg := &Config{
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
	}
	if err := cfg.validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

// validate проверяет, что URL'ы имеют корректную схему и хост. Это защищает
// от SSRF, когда атакующий контролирует env (например, multi-tenant runtime).
func (c *Config) validate() error {
	if err := requireHTTPURL("OPENCODE_BASE_URL", c.OpenCodeBaseURL); err != nil {
		return err
	}
	if err := requireHTTPURL("HA_URL", c.HAURL); err != nil {
		return err
	}
	return nil
}

func requireHTTPURL(name, raw string) error {
	if raw == "" {
		return fmt.Errorf("%s is empty", name)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s: parse: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s: scheme must be http or https, got %q", name, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%s: empty host in %q", name, raw)
	}
	return nil
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
