import { describe, it, expect } from 'vitest'
import {
  filterTorrents,
  sortTorrents,
  encodeViewState,
  decodeViewState,
  DEFAULT_VIEW_STATE,
} from '../../src/lib/utils/torrentFilters'
import type { SortKey, SortDir, ViewState } from '../../src/lib/utils/torrentFilters'
import type { TorrentInfo } from '../../src/lib/api/client'

function makeTorrent(overrides: Partial<TorrentInfo> = {}): TorrentInfo {
  return {
    hash: 'h1',
    name: 'Some Torrent',
    status: 'downloading',
    completed: false,
    anime_name: undefined,
    anime_id: undefined,
    episode_number: null,
    is_batch: false,
    bytes_completed: 0,
    bytes_total: 100,
    bytes_uploaded: 0,
    progress: 0,
    download_speed: 0,
    upload_speed: 0,
    peers_total: 0,
    eta_seconds: null,
    seeded_for_seconds: 0,
    ...overrides,
  }
}

describe('filterTorrents', () => {
  it('matches query against anime_name regardless of accent/case', () => {
    const list = [makeTorrent({ hash: 'a', anime_name: 'Frieren' })]
    expect(filterTorrents(list, { query: 'FRIEREN', statuses: [] })).toHaveLength(1)
    expect(filterTorrents(list, { query: 'frieren', statuses: [] })).toHaveLength(1)
  })

  it('matches query with accents against accented text', () => {
    const list = [makeTorrent({ hash: 'a', anime_name: 'Attaque des Titans' })]
    expect(filterTorrents(list, { query: 'attaque', statuses: [] })).toHaveLength(1)
  })

  it('matches query against the raw torrent name too', () => {
    const list = [makeTorrent({ hash: 'a', name: '[SubsPlease] Frieren - 01.mkv', anime_name: undefined })]
    expect(filterTorrents(list, { query: 'subsplease', statuses: [] })).toHaveLength(1)
  })

  it('empty query means no filtering', () => {
    const list = [makeTorrent({ hash: 'a' }), makeTorrent({ hash: 'b' })]
    expect(filterTorrents(list, { query: '', statuses: [] })).toHaveLength(2)
  })

  it('excludes non-matching entries', () => {
    const list = [makeTorrent({ hash: 'a', anime_name: 'Frieren' }), makeTorrent({ hash: 'b', anime_name: 'Bleach' })]
    expect(filterTorrents(list, { query: 'bleach', statuses: [] })).toEqual([list[1]])
  })

  it('filters by multiple status slugs', () => {
    const list = [
      makeTorrent({ hash: 'a', status: 'downloading' }),
      makeTorrent({ hash: 'b', status: 'seeding' }),
      makeTorrent({ hash: 'c', status: 'stopped' }),
    ]
    const result = filterTorrents(list, { query: '', statuses: ['downloading', 'seeding'] })
    expect(result.map((t) => t.hash)).toEqual(['a', 'b'])
  })

  it('empty statuses list means no filtering', () => {
    const list = [makeTorrent({ hash: 'a', status: 'downloading' }), makeTorrent({ hash: 'b', status: 'stopped' })]
    expect(filterTorrents(list, { query: '', statuses: [] })).toHaveLength(2)
  })
})

describe('sortTorrents', () => {
  it('preserves received order for sort: default, ignoring dir', () => {
    const list = [makeTorrent({ hash: 'b' }), makeTorrent({ hash: 'a' }), makeTorrent({ hash: 'c' })]
    expect(sortTorrents(list, 'default', 'asc').map((t) => t.hash)).toEqual(['b', 'a', 'c'])
    expect(sortTorrents(list, 'default', 'desc').map((t) => t.hash)).toEqual(['b', 'a', 'c'])
  })

  it('does not mutate the input array', () => {
    const list = [makeTorrent({ hash: 'b', name: 'B' }), makeTorrent({ hash: 'a', name: 'A' })]
    const copy = list.slice()
    sortTorrents(list, 'name', 'asc')
    expect(list).toEqual(copy)
  })

  it('sorts by name asc/desc', () => {
    const list = [makeTorrent({ hash: 'b', name: 'Bravo' }), makeTorrent({ hash: 'a', name: 'Alpha' })]
    expect(sortTorrents(list, 'name', 'asc').map((t) => t.hash)).toEqual(['a', 'b'])
    expect(sortTorrents(list, 'name', 'desc').map((t) => t.hash)).toEqual(['b', 'a'])
  })

  it('sorts by progress asc/desc', () => {
    const list = [makeTorrent({ hash: 'a', progress: 0.9 }), makeTorrent({ hash: 'b', progress: 0.1 })]
    expect(sortTorrents(list, 'progress', 'asc').map((t) => t.hash)).toEqual(['b', 'a'])
    expect(sortTorrents(list, 'progress', 'desc').map((t) => t.hash)).toEqual(['a', 'b'])
  })

  it('sorts by download_speed asc/desc', () => {
    const list = [makeTorrent({ hash: 'a', download_speed: 500 }), makeTorrent({ hash: 'b', download_speed: 100 })]
    expect(sortTorrents(list, 'download_speed', 'asc').map((t) => t.hash)).toEqual(['b', 'a'])
    expect(sortTorrents(list, 'download_speed', 'desc').map((t) => t.hash)).toEqual(['a', 'b'])
  })

  it('sorts by size asc/desc', () => {
    const list = [makeTorrent({ hash: 'a', bytes_total: 5000 }), makeTorrent({ hash: 'b', bytes_total: 100 })]
    expect(sortTorrents(list, 'size', 'asc').map((t) => t.hash)).toEqual(['b', 'a'])
    expect(sortTorrents(list, 'size', 'desc').map((t) => t.hash)).toEqual(['a', 'b'])
  })

  it('sorts by peers asc/desc', () => {
    const list = [makeTorrent({ hash: 'a', peers_total: 20 }), makeTorrent({ hash: 'b', peers_total: 3 })]
    expect(sortTorrents(list, 'peers', 'asc').map((t) => t.hash)).toEqual(['b', 'a'])
    expect(sortTorrents(list, 'peers', 'desc').map((t) => t.hash)).toEqual(['a', 'b'])
  })

  it('sorts by eta asc/desc with nulls always last', () => {
    const list = [
      makeTorrent({ hash: 'a', eta_seconds: null }),
      makeTorrent({ hash: 'b', eta_seconds: 300 }),
      makeTorrent({ hash: 'c', eta_seconds: 10 }),
    ]
    expect(sortTorrents(list, 'eta', 'asc').map((t) => t.hash)).toEqual(['c', 'b', 'a'])
    expect(sortTorrents(list, 'eta', 'desc').map((t) => t.hash)).toEqual(['b', 'c', 'a'])
  })
})

describe('encodeViewState / decodeViewState', () => {
  it('encodes a clean default state as an empty querystring', () => {
    expect(encodeViewState(DEFAULT_VIEW_STATE)).toBe('')
  })

  it('decodes an empty querystring as the default state', () => {
    expect(decodeViewState('')).toEqual(DEFAULT_VIEW_STATE)
  })

  it('encodes non-default fields', () => {
    const v: ViewState = { query: 'frieren', statuses: ['downloading', 'seeding'], sort: 'progress', dir: 'desc' }
    const qs = encodeViewState(v)
    expect(qs).toContain('q=frieren')
    expect(qs).toContain('status=downloading%2Cseeding')
    expect(qs).toContain('sort=progress')
    expect(qs).toContain('dir=desc')
  })

  it('omits dir when sort is default', () => {
    const v: ViewState = { query: 'frieren', statuses: [], sort: 'default', dir: 'desc' }
    expect(encodeViewState(v)).not.toContain('dir=')
  })

  it('round-trips a valid non-default state', () => {
    const v: ViewState = { query: 'bleach', statuses: ['stopped', 'verifying'], sort: 'eta', dir: 'asc' }
    expect(decodeViewState(encodeViewState(v))).toEqual(v)
  })

  it('round-trips the default state', () => {
    expect(decodeViewState(encodeViewState(DEFAULT_VIEW_STATE))).toEqual(DEFAULT_VIEW_STATE)
  })

  it('falls back to defaults for garbage sort/dir/status values', () => {
    const result = decodeViewState('q=abc&status=downloading,not_a_slug,seeding&sort=bogus&dir=sideways')
    expect(result.query).toBe('abc')
    expect(result.statuses).toEqual(['downloading', 'seeding'])
    expect(result.sort).toBe('default' satisfies SortKey)
    expect(result.dir).toBe('asc' satisfies SortDir)
  })

  it('falls back to default dir when garbage but sort is valid', () => {
    const result = decodeViewState('sort=name&dir=sideways')
    expect(result.sort).toBe('name')
    expect(result.dir).toBe('asc')
  })
})
