#!/bin/sh
set -e

CONFIG_DIR="/app/data/.autoAnimeDownloader"
CONFIG_FILE="$CONFIG_DIR/config.json"

# Criar diretório se não existir
mkdir -p "$CONFIG_DIR"

# Valores padrão
ANILIST_USERNAME="${ANILIST_USERNAME:-}"
SAVE_PATH="${SAVE_PATH:-/app/downloads}"
COMPLETED_ANIME_PATH="${COMPLETED_ANIME_PATH:-/app/downloads/completed}"
CHECK_INTERVAL="${CHECK_INTERVAL:-10}"
QBITTORRENT_URL="${QBITTORRENT_URL:-http://qbittorrent:8080}"
MAX_EPISODES_PER_ANIME="${MAX_EPISODES_PER_ANIME:-12}"
EPISODE_RETRY_LIMIT="${EPISODE_RETRY_LIMIT:-5}"
DELETE_WATCHED_EPISODES="${DELETE_WATCHED_EPISODES:-true}"
EXCLUDED_LIST="${EXCLUDED_LIST:-}"

# Converter DELETE_WATCHED_EPISODES para boolean JSON
if [ "$DELETE_WATCHED_EPISODES" = "true" ] || [ "$DELETE_WATCHED_EPISODES" = "1" ]; then
    DELETE_WATCHED_EPISODES_JSON="true"
else
    DELETE_WATCHED_EPISODES_JSON="false"
fi

# Criar ou atualizar config.json
if [ ! -f "$CONFIG_FILE" ] || [ -n "$FORCE_CONFIG_UPDATE" ]; then
    cat > "$CONFIG_FILE" <<EOF
{
  "save_path": "$SAVE_PATH",
  "completed_anime_path": "$COMPLETED_ANIME_PATH",
  "anilist_username": "$ANILIST_USERNAME",
  "check_interval": $CHECK_INTERVAL,
  "qbittorrent_url": "$QBITTORRENT_URL",
  "max_episodes_per_anime": $MAX_EPISODES_PER_ANIME,
  "episode_retry_limit": $EPISODE_RETRY_LIMIT,
  "delete_watched_episodes": $DELETE_WATCHED_EPISODES_JSON,
  "excluded_list": "$EXCLUDED_LIST"
}
EOF
    echo "Config file created/updated at $CONFIG_FILE"
else
    echo "Config file already exists at $CONFIG_FILE, skipping..."
fi

# Executar o comando original
exec "$@"

