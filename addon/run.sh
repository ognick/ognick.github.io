#!/usr/bin/env sh
set -e

OPTIONS=/data/options.json

if [ -f "$OPTIONS" ]; then
    LOG_LEVEL=$(jq -r '.log_level // "info"' "$OPTIONS")
    OPENCODE_API_KEY=$(jq -r '.opencode_api_key // ""' "$OPTIONS")
    OPENCODE_BASE_URL=$(jq -r '.opencode_base_url // "https://opencode.ai/zen/go/v1"' "$OPTIONS")
    OPENCODE_MODEL=$(jq -r '.opencode_model // "deepseek-v4-flash"' "$OPTIONS")
    ALLOWED_EMAILS=$(jq -r '.allowed_emails // [] | join(",")' "$OPTIONS")
    HA_TOKEN=$(jq -r '.ha_token // ""' "$OPTIONS")
    HA_URL=$(jq -r '.ha_url // "http://homeassistant:8123"' "$OPTIONS")
    POLICY_CACHE_TTL_SECONDS=$(jq -r '.policy_cache_ttl_seconds // 60' "$OPTIONS")
    YOUTUBE_API_KEY=$(jq -r '.youtube_api_key // ""' "$OPTIONS")
else
    LOG_LEVEL="${LOG_LEVEL:-info}"
    OPENCODE_API_KEY="${OPENCODE_API_KEY:-}"
    OPENCODE_BASE_URL="${OPENCODE_BASE_URL:-https://opencode.ai/zen/go/v1}"
    OPENCODE_MODEL="${OPENCODE_MODEL:-deepseek-v4-flash}"
    ALLOWED_EMAILS="${ALLOWED_EMAILS:-}"
    HA_TOKEN="${HA_TOKEN:-}"
    HA_URL="${HA_URL:-http://homeassistant:8123}"
    POLICY_CACHE_TTL_SECONDS="${POLICY_CACHE_TTL_SECONDS:-60}"
    YOUTUBE_API_KEY="${YOUTUBE_API_KEY:-}"
fi

export ADDR=":8080"
export DB_PATH="/data/zabkiss.db"
export LOG_LEVEL
export OPENCODE_API_KEY
export OPENCODE_BASE_URL
export OPENCODE_MODEL
export ALLOWED_EMAILS
export HA_TOKEN
export HA_URL
export POLICY_CACHE_TTL_SECONDS
export YOUTUBE_API_KEY

exec /usr/bin/zabkiss
