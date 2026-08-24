#!/bin/sh
set -e

CONFIG_DIR="/app/data/.autoAnimeDownloader"
CONFIG_FILE="$CONFIG_DIR/config.json"

# Criar diretório se não existir
mkdir -p "$CONFIG_DIR"

# csv_to_json_array "a, b ,c" -> ["a","b","c"]; "" -> []
# anilist_usernames e excluded_lists sao arrays no config.json; as env vars sao CSV.
csv_to_json_array() {
    echo "$1" | awk -F, '{
        out = ""; n = 0
        for (i = 1; i <= NF; i++) {
            gsub(/^[ \t]+|[ \t]+$/, "", $i)
            if ($i == "") continue
            gsub(/\\/, "\\\\", $i); gsub(/"/, "\\\"", $i)
            if (n++) out = out ","
            out = out "\"" $i "\""
        }
        printf "[%s]", out
    }'
}

# Valores padrão. ANILIST_USERNAME/EXCLUDED_LIST (singular) sao aceitos como legado das
# imagens antigas, mas o config.json gerado usa sempre os campos plurais atuais.
ANILIST_USERNAMES="${ANILIST_USERNAMES:-${ANILIST_USERNAME:-}}"
COMPLETED_ANIME_PATH="${COMPLETED_ANIME_PATH:-/app/downloads/completed}"
CHECK_INTERVAL="${CHECK_INTERVAL:-10}"
MAX_EPISODES_PER_ANIME="${MAX_EPISODES_PER_ANIME:-12}"
EPISODE_RETRY_LIMIT="${EPISODE_RETRY_LIMIT:-5}"
DELETE_WATCHED_EPISODES="${DELETE_WATCHED_EPISODES:-true}"
EXCLUDED_LISTS="${EXCLUDED_LISTS:-${EXCLUDED_LIST:-}}"

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
  "completed_anime_path": "$COMPLETED_ANIME_PATH",
  "anilist_usernames": $(csv_to_json_array "$ANILIST_USERNAMES"),
  "check_interval": $CHECK_INTERVAL,
  "max_episodes_per_anime": $MAX_EPISODES_PER_ANIME,
  "episode_retry_limit": $EPISODE_RETRY_LIMIT,
  "delete_watched_episodes": $DELETE_WATCHED_EPISODES_JSON,
  "excluded_lists": $(csv_to_json_array "$EXCLUDED_LISTS")
}
EOF
    echo "Config file created/updated at $CONFIG_FILE"
else
    echo "Config file already exists at $CONFIG_FILE, skipping..."
fi

# Executar o comando original
exec "$@"

