# After the search: the embedded client and organize

For the [debug-anime](SKILL.md) cases where a magnet was found but the episode still never reached
the library. The BitTorrent client is embedded (`github.com/cenkalti/rain/v2`): there is no
qBittorrent to connect to and no `qbittorrent_url` config.

- **Resume database:** rain keeps resume data (piece bitfields) in a bbolt DB at
  `~/.autoAnimeDownloader/session.db` (Windows: `%APPDATA%\.autoAnimeDownloader\session.db`),
  outside the download path so it survives a `completed_anime_path` change. Torrents not resuming
  after a restart → check the file exists and is writable.
- **Ports / connectivity:** rain listens on a port range (20000–30000), with **no UPnP/NAT-PMP**.
  Inbound peers may need manual forwarding, but DHT + PEX still find peers without it. A
  well-seeded torrent with zero peers → suspect a firewall on that range or no outbound
  connectivity. There is no `torrent_port` config.
- **Downloaded but not in the library:** completion queues an `organize` job that **hardlinks** the
  video files into `completed_anime_path`.

  ```bash
  grep '"Organize:' ~/.autoAnimeDownloader/daemon.log
  cat ~/.autoAnimeDownloader/pending_jobs.json   # a stuck organize job sits here, retrying
  ```

  The download path is a hidden subfolder of `completed_anime_path`, so both are on the same volume
  by construction — the old cross-device hardlink failure can't happen. The usual cause today is
  `"no video files found in <dir>"`: the torrent has no recognized video file.
