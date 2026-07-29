import type { AnimeEpisodeInfo, TorrentInfo } from '../api/client.js'

/**
 * Indexes torrents by episode number via `episode.episode_hash === torrent.hash`.
 *
 * A batch torrent's hash is shared by every episode it covers, so it appears in the map
 * under each of those episode numbers. Episodes without an `episode_hash`, and torrents
 * whose hash doesn't match any episode (orphans, or torrents belonging to another anime),
 * are simply absent from the result.
 */
export function indexTorrentsByEpisode(
  torrents: TorrentInfo[],
  episodes: AnimeEpisodeInfo[],
): Map<number, TorrentInfo> {
  const torrentsByHash = new Map<string, TorrentInfo>()
  for (const t of torrents) {
    torrentsByHash.set(t.hash, t)
  }

  const result = new Map<number, TorrentInfo>()
  for (const episode of episodes) {
    if (!episode.episode_hash) continue
    const torrent = torrentsByHash.get(episode.episode_hash)
    if (!torrent) continue
    result.set(episode.episode_number, torrent)
  }
  return result
}
