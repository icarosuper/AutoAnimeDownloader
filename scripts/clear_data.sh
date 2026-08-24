#!/bin/bash
# Wipes the daemon's local STATE, keeping config.json (the user's settings).
# Every file here is written under ~/.autoAnimeDownloader by files/filemanager.go,
# cmd/daemon/main.go, logger/logger.go or torrents/{queue,rootmarker}.go.
#
# Not cleared: config.json, and the download folder's own .aad_root marker (it lives
# inside the library, not here — delete the folder itself if you want a full reset).

set -u

DIR="${HOME}/.autoAnimeDownloader"

for f in \
	daemon.log \
	downloaded_episodes \
	blocked_episodes \
	anime_settings \
	standalone_animes \
	pending_jobs.json \
	session.db \
	queue.json \
	download_root.id
do
	rm -f "$DIR/$f"
done

echo "Cleared daemon state in $DIR (config.json kept)."
