#!/bin/sh
set -e

CONFIG_DIR="/app/data/.autoAnimeDownloader"
CONFIG_FILE="$CONFIG_DIR/config.json"

# Create the directory if it does not exist
mkdir -p "$CONFIG_DIR"

# csv_to_json_array "a, b ,c" -> ["a","b","c"]; "" -> []
# anilist_usernames, mal_usernames and excluded_lists are arrays in config.json; the env vars are CSV.
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

# Defaults. ANILIST_USERNAME/EXCLUDED_LIST (singular) are accepted as a legacy fallback from
# older images, but the generated config.json always uses the current plural keys.
ANILIST_USERNAMES="${ANILIST_USERNAMES:-${ANILIST_USERNAME:-}}"
MAL_USERNAMES="${MAL_USERNAMES:-}"
COMPLETED_ANIME_PATH="${COMPLETED_ANIME_PATH:-/app/downloads/completed}"
CHECK_INTERVAL="${CHECK_INTERVAL:-10}"
MAX_EPISODES_PER_ANIME="${MAX_EPISODES_PER_ANIME:-12}"
EPISODE_RETRY_LIMIT="${EPISODE_RETRY_LIMIT:-5}"
DELETE_WATCHED_EPISODES="${DELETE_WATCHED_EPISODES:-true}"
EXCLUDED_LISTS="${EXCLUDED_LISTS:-${EXCLUDED_LIST:-}}"

# Convert DELETE_WATCHED_EPISODES to a JSON boolean
if [ "$DELETE_WATCHED_EPISODES" = "true" ] || [ "$DELETE_WATCHED_EPISODES" = "1" ]; then
    DELETE_WATCHED_EPISODES_JSON="true"
else
    DELETE_WATCHED_EPISODES_JSON="false"
fi

# Create or update config.json
if [ ! -f "$CONFIG_FILE" ] || [ -n "$FORCE_CONFIG_UPDATE" ]; then
    cat > "$CONFIG_FILE" <<EOF
{
  "completed_anime_path": "$COMPLETED_ANIME_PATH",
  "anilist_usernames": $(csv_to_json_array "$ANILIST_USERNAMES"),
  "mal_usernames": $(csv_to_json_array "$MAL_USERNAMES"),
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

# Run the original command
exec "$@"

