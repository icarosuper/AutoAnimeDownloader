import { describe, it, expect } from 'vitest'
import {
  filterTorrents,
  sortTorrents,
  encodeViewState,
  decodeViewState,
  groupKey,
  groupTorrents,
  isProblemTorrent,
  prioritizeOrder,
  selectionPrioritizeOrder,
  DEFAULT_VIEW_STATE,
} from '../../src/lib/utils/torrentFilters'
import type { SortKey, SortDir, ViewState } from '../../src/lib/utils/torrentFilters'
import type { TorrentInfo } from '../../src/lib/api/client'

function makeTorrent(overrides: Partial<TorrentInfo> = {}): TorrentInfo {
  return {
    hash: 'h1',
    name: 'Some Torrent',
    status: 'downloading',
    queue_position: 0,
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
    const v: ViewState = { query: 'frieren', statuses: ['downloading', 'seeding'], problems: false, sort: 'progress', dir: 'desc', closed: [] }
    const qs = encodeViewState(v)
    expect(qs).toContain('q=frieren')
    expect(qs).toContain('status=downloading%2Cseeding')
    expect(qs).toContain('sort=progress')
    expect(qs).toContain('dir=desc')
  })

  it('omits dir when sort is default', () => {
    const v: ViewState = { query: 'frieren', statuses: [], problems: false, sort: 'default', dir: 'desc', closed: [] }
    expect(encodeViewState(v)).not.toContain('dir=')
  })

  it('round-trips a valid non-default state', () => {
    const v: ViewState = { query: 'bleach', statuses: ['stopped', 'verifying'], problems: true, sort: 'eta', dir: 'asc', closed: ['a:1', 'n:Bleach'] }
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

describe('isProblemTorrent', () => {
  it('flags an active torrent with no peers', () => {
    expect(isProblemTorrent(makeTorrent({ status: 'downloading', peers_total: 0 }))).toBe(true)
  })

  it('does not flag a torrent that has peers', () => {
    expect(isProblemTorrent(makeTorrent({ status: 'downloading', peers_total: 4 }))).toBe(false)
  })

  // Pausing is a user decision, not a fault — the pill would otherwise fire constantly for
  // anyone who pauses things deliberately.
  it('does not flag a paused torrent even at zero peers', () => {
    expect(isProblemTorrent(makeTorrent({ status: 'stopped', peers_total: 0 }))).toBe(false)
    expect(isProblemTorrent(makeTorrent({ status: 'stopping', peers_total: 0 }))).toBe(false)
  })

  // A finished download nobody is pulling from is a quiet seed, not a broken transfer.
  it('does not flag a completed torrent at zero peers', () => {
    expect(isProblemTorrent(makeTorrent({ status: 'seeding', completed: true, peers_total: 0 }))).toBe(false)
  })
})

describe('filterTorrents with the problems flag', () => {
  const list = [
    makeTorrent({ hash: 'ok', status: 'downloading', peers_total: 9 }),
    makeTorrent({ hash: 'stuck', status: 'downloading', peers_total: 0 }),
    makeTorrent({ hash: 'paused', status: 'stopped', peers_total: 0 }),
  ]

  it('keeps only problem torrents when set', () => {
    const result = filterTorrents(list, { query: '', statuses: [], problems: true })
    expect(result.map((t) => t.hash)).toEqual(['stuck'])
  })

  it('is a no-op when unset or omitted', () => {
    expect(filterTorrents(list, { query: '', statuses: [], problems: false })).toHaveLength(3)
    expect(filterTorrents(list, { query: '', statuses: [] })).toHaveLength(3)
  })
})

describe('groupKey', () => {
  it('prefers anime_id', () => {
    expect(groupKey(makeTorrent({ anime_id: 7, anime_name: 'Frieren' }))).toBe('a:7')
  })

  it('falls back to anime_name when there is no id', () => {
    expect(groupKey(makeTorrent({ anime_name: 'Frieren' }))).toBe('n:Frieren')
  })

  it('buckets everything unlinked into one group', () => {
    expect(groupKey(makeTorrent({}))).toBe('other')
  })

  // The prefixes exist so a numeric id can't collide with an anime literally named "7".
  it('does not collide an id with a same-looking name', () => {
    expect(groupKey(makeTorrent({ anime_id: 7 }))).not.toBe(groupKey(makeTorrent({ anime_name: '7' })))
  })
})

describe('groupTorrents', () => {
  it('buckets by anime and sums speeds', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: '1', anime_id: 1, anime_name: 'Alpha', download_speed: 100, upload_speed: 10 }),
      makeTorrent({ hash: '2', anime_id: 1, anime_name: 'Alpha', download_speed: 50, upload_speed: 5 }),
      makeTorrent({ hash: '3', anime_id: 2, anime_name: 'Beta', peers_total: 3 }),
    ])

    expect(groups).toHaveLength(2)
    const alpha = groups.find((g) => g.key === 'a:1')!
    expect(alpha.torrents).toHaveLength(2)
    expect(alpha.downloadSpeed).toBe(150)
    expect(alpha.uploadSpeed).toBe(15)
  })

  it('orders problems first, then downloading, then the rest', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: 's', anime_id: 3, anime_name: 'Seeding one', status: 'seeding', completed: true, peers_total: 0 }),
      makeTorrent({ hash: 'd', anime_id: 2, anime_name: 'Downloading one', status: 'downloading', peers_total: 5 }),
      makeTorrent({ hash: 'p', anime_id: 1, anime_name: 'Problem one', status: 'downloading', peers_total: 0 }),
    ])

    expect(groups.map((g) => g.name)).toEqual(['Problem one', 'Downloading one', 'Seeding one'])
  })

  it('breaks ties alphabetically within the same rank', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: 'z', anime_id: 1, anime_name: 'Zeta', status: 'downloading', peers_total: 2 }),
      makeTorrent({ hash: 'a', anime_id: 2, anime_name: 'Alpha', status: 'downloading', peers_total: 2 }),
    ])
    expect(groups.map((g) => g.name)).toEqual(['Alpha', 'Zeta'])
  })

  // rain frees piece data on stop, so bytes_completed goes to 0 while `progress` stays honest.
  // A sum(bytes_completed)/sum(bytes_total) aggregate would collapse the bar on pause.
  it('weights progress by bytes_total and ignores bytes_completed', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: 'big', anime_id: 1, anime_name: 'A', progress: 1, bytes_total: 300, bytes_completed: 0, peers_total: 2 }),
      makeTorrent({ hash: 'small', anime_id: 1, anime_name: 'A', progress: 0, bytes_total: 100, bytes_completed: 0, peers_total: 2 }),
    ])
    expect(groups[0].progress).toBeCloseTo(0.75)
  })

  it('falls back to a plain mean when no torrent has metadata yet', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: 'a', anime_id: 1, anime_name: 'A', progress: 0.4, bytes_total: 0, peers_total: 2 }),
      makeTorrent({ hash: 'b', anime_id: 1, anime_name: 'A', progress: 0.8, bytes_total: 0, peers_total: 2 }),
    ])
    expect(groups[0].progress).toBeCloseTo(0.6)
  })

  it('counts problems and downloading torrents per group', () => {
    const groups = groupTorrents([
      makeTorrent({ hash: '1', anime_id: 1, anime_name: 'A', status: 'downloading', peers_total: 0 }),
      makeTorrent({ hash: '2', anime_id: 1, anime_name: 'A', status: 'downloading', peers_total: 3 }),
    ])
    expect(groups[0].problemCount).toBe(1)
    expect(groups[0].downloadingCount).toBe(2)
  })
})

describe('accordion state in the querystring', () => {
  it('encodes collapsed groups', () => {
    expect(encodeViewState({ ...DEFAULT_VIEW_STATE, closed: ['a:1', 'a:2'] })).toContain('closed=a%3A1%2Ca%3A2')
  })

  // Everything expanded is the default, so it must serialize to a clean URL.
  it('omits closed when nothing is collapsed', () => {
    expect(encodeViewState(DEFAULT_VIEW_STATE)).toBe('')
  })

  it('round-trips collapsed group keys', () => {
    expect(decodeViewState('closed=a:1,n:Bleach').closed).toEqual(['a:1', 'n:Bleach'])
  })

  it('drops empty entries but keeps keys for groups that no longer exist', () => {
    expect(decodeViewState('closed=a:1,,%20,a:999').closed).toEqual(['a:1', 'a:999'])
  })

  it('encodes and decodes the problems flag', () => {
    expect(encodeViewState({ ...DEFAULT_VIEW_STATE, problems: true })).toContain('problems=1')
    expect(decodeViewState('problems=1').problems).toBe(true)
    expect(decodeViewState('problems=yes').problems).toBe(false)
    expect(decodeViewState('').problems).toBe(false)
  })
})

describe('prioritizeOrder', () => {
  it('sorts by episode_number regardless of the order shown', () => {
    const list = [
      makeTorrent({ hash: 'e3', episode_number: 3 }),
      makeTorrent({ hash: 'e1', episode_number: 1 }),
      makeTorrent({ hash: 'e2', episode_number: 2 }),
    ]
    expect(prioritizeOrder(list)).toEqual(['e1', 'e2', 'e3'])
  })

  it('sends batches and numberless torrents last, in display order', () => {
    const list = [
      makeTorrent({ hash: 'batch', is_batch: true, episode_number: null }),
      makeTorrent({ hash: 'e2', episode_number: 2 }),
      makeTorrent({ hash: 'other', episode_number: null }),
      makeTorrent({ hash: 'e1', episode_number: 1 }),
    ]
    expect(prioritizeOrder(list)).toEqual(['e1', 'e2', 'batch', 'other'])
  })
})

describe('selectionPrioritizeOrder', () => {
  it('keeps the click order, not the display order', () => {
    const visible = [
      makeTorrent({ hash: 'a', status: 'stopped' }),
      makeTorrent({ hash: 'b', status: 'stopped' }),
      makeTorrent({ hash: 'c', status: 'stopped' }),
    ]
    const selected = new Set(['c', 'a'])
    expect(selectionPrioritizeOrder(selected, visible, () => true)).toEqual(['c', 'a'])
  })

  it('drops hashes that left the list or cannot be prioritized', () => {
    const visible = [
      makeTorrent({ hash: 'a', status: 'stopped' }),
      makeTorrent({ hash: 'done', status: 'seeding', completed: true }),
    ]
    const selected = new Set(['gone', 'done', 'a'])
    expect(selectionPrioritizeOrder(selected, visible, (t) => !t.completed)).toEqual(['a'])
  })
})
