import type { TorrentFile } from '../api/client.js'

/**
 * Files that map to an episode, one per number, sorted by number.
 *
 * Only `episode != null` survives: what the heuristic could not match has no place on a screen
 * organized by episode (the full, raw list is one click away on Downloads).
 *
 * Two files claiming the same number — `NCOP 01.mkv` matches the ` 05.mkv` pattern and reads as
 * "Ep 01" — resolve by LARGEST FILE WINS. Without that a 200 MB NCOP steals episode 1's bar
 * from a 1.4 GB episode.
 */
export function filesByEpisode(files: TorrentFile[]): TorrentFile[] {
  const best = new Map<number, TorrentFile>()
  for (const f of files) {
    if (f.episode === null) continue
    const current = best.get(f.episode)
    if (!current || f.size > current.size) best.set(f.episode, f)
  }
  return [...best.values()].sort((a, b) => (a.episode ?? 0) - (b.episode ?? 0))
}
