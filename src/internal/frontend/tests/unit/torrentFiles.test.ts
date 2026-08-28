import { describe, it, expect } from 'vitest'
import { filesByEpisode } from '../../src/lib/utils/torrentFiles.js'
import type { TorrentFile } from '../../src/lib/api/client.js'

const file = (path: string, episode: number | null, size = 1000): TorrentFile => ({
  path,
  size,
  bytes_completed: null,
  episode,
})

describe('filesByEpisode', () => {
  it('drops files the episode heuristic did not match', () => {
    const out = filesByEpisode([file('ep01.mkv', 1), file('readme.txt', null)])
    expect(out.map((f) => f.path)).toEqual(['ep01.mkv'])
  })

  it('keeps the largest file when two claim the same episode', () => {
    const out = filesByEpisode([
      file('NCOP 01.mkv', 1, 200_000_000),
      file('[Judas] Frieren - 01.mkv', 1, 1_400_000_000),
    ])
    expect(out).toHaveLength(1)
    expect(out[0].path).toBe('[Judas] Frieren - 01.mkv')
  })

  it('sorts by episode number, not by the torrent order', () => {
    const out = filesByEpisode([file('c.mkv', 3), file('a.mkv', 1), file('b.mkv', 2)])
    expect(out.map((f) => f.episode)).toEqual([1, 2, 3])
  })
})
