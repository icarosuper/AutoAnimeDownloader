import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Status from '../../src/routes/Status.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AnimeInfo, StatusResponse } from '../../src/lib/api/client.js'

vi.mock('../../src/lib/api/client.js', () => ({
  getStatus: vi.fn(),
  getAnimes: vi.fn(),
  getConfig: vi.fn(),
  triggerCheck: vi.fn(),
  startDaemon: vi.fn(),
  stopDaemon: vi.fn(),
  getTorrents: vi.fn(),
  getLastCheck: vi.fn(),
}))

vi.mock('../../src/lib/websocket/client.js', () => ({
  WebSocketClient: class {
    connect() {}
    disconnect() {}
  },
}))

function anime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Alpha',
    episodes_downloaded: 0,
    episodes_released: 12,
    episodes_watched: 0,
    episodes_pending: 12,
    total_episodes: 12,
    latest_episode_number: 1,
    last_download_date: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const library: AnimeInfo[] = [
  anime({ anime_id: 1, name: 'Airing Show', media_status: 'RELEASING', episodes_watched: 1, episodes_released: 8 }),
  anime({ anime_id: 2, name: 'Finished Show', media_status: 'FINISHED', episodes_watched: 12, episodes_released: 12 }),
  anime({ anime_id: 3, name: 'Finished Leftover', media_status: 'FINISHED', episodes_watched: 2, episodes_released: 12 }),
  anime({ anime_id: 4, name: 'Standalone Show', media_status: 'RELEASING', is_standalone: true }),
  anime({ anime_id: 5, name: 'Blacklisted Show', media_status: 'FINISHED', is_blacklisted: true }),
]

async function renderStatus(animes: AnimeInfo[] = library, config: Record<string, unknown> | Error = {}) {
  vi.mocked(client.getStatus).mockResolvedValue({
    status: 'running',
    last_check: '2026-08-19T12:00:00Z',
    has_error: false,
    version: '1.0.0',
    disk_total: 100,
    disk_free: 90,
    disk_low: false,
  } as StatusResponse)
  vi.mocked(client.getAnimes).mockResolvedValue(animes)
  if (config instanceof Error) vi.mocked(client.getConfig).mockRejectedValue(config)
  else vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60, ...config } as never)
  vi.mocked(client.getTorrents).mockResolvedValue([])
  vi.mocked(client.getLastCheck).mockResolvedValue({
    finished_at: '0001-01-01T00:00:00Z',
    pass_error: '',
    problems: [],
    limits: [],
  })
  const view = render(Status)
  await tick()
  await tick()
  await tick()
  return view
}

describe('Status — type filters', () => {
  beforeEach(() => {
    window.location.hash = '#/status'
  })

  it('keeps the type radiogroup and the type ActionMenu in the DOM together', async () => {
    await renderStatus()
    expect(screen.getByRole('radiogroup', { name: /filter by type|filtrar por tipo/i })).toBeTruthy()
    expect(screen.getByRole('button', { name: /^(all|tudo) /i })).toBeTruthy()
  })

  it('clicking Airing filters the table and All restores it', async () => {
    await renderStatus()
    await fireEvent.click(screen.getAllByRole('radio', { name: /airing|saindo/i })[0])
    await tick()
    expect(screen.getAllByText('Airing Show').length).toBeGreaterThan(0)
    expect(screen.queryByText('Finished Show')).toBeNull()

    await fireEvent.click(screen.getAllByRole('radio', { name: /^(all|tudo) /i })[0])
    await tick()
    expect(screen.getAllByText('Finished Show').length).toBeGreaterThan(0)
  })

  it('combines Unwatched with Airing and shows matching counts', async () => {
    await renderStatus()
    await fireEvent.click(screen.getAllByRole('radio', { name: /airing|saindo/i })[0])
    await fireEvent.click(screen.getByRole('button', { name: /unwatched|não vistos/i }))
    await tick()
    expect(screen.getAllByText('Airing Show').length).toBeGreaterThan(0)
    expect(screen.queryByText('Finished Leftover')).toBeNull()
    expect(screen.getByText(/2 of 5|2 de 5/)).toBeTruthy()
  })

  it('shows N of M and the clear control only when a filter is active, and the X resets everything', async () => {
    await renderStatus()
    expect(screen.queryByRole('button', { name: /clear filters|limpar filtros/i })).toBeNull()
    expect(screen.queryByText(/of 5|de 5/)).toBeNull()

    await fireEvent.click(screen.getAllByRole('radio', { name: /finished|finalizados/i })[0])
    await tick()
    expect(screen.getByRole('button', { name: /clear filters|limpar filtros/i })).toBeTruthy()
    expect(screen.getByText(/3 of 5|3 de 5/)).toBeTruthy()

    await fireEvent.click(screen.getByRole('button', { name: /clear filters|limpar filtros/i }))
    await tick()
    expect(screen.queryByRole('button', { name: /clear filters|limpar filtros/i })).toBeNull()
    expect(screen.getAllByText('Airing Show').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Finished Show').length).toBeGreaterThan(0)
  })

  it('keeps the clear X in the same cluster as N of M, not as a sibling of the heading', async () => {
    await renderStatus()
    await fireEvent.click(screen.getAllByRole('radio', { name: /finished|finalizados/i })[0])
    await tick()
    const count = screen.getByText(/3 of 5|3 de 5/)
    const clear = screen.getByRole('button', { name: /clear filters|limpar filtros/i })
    const cluster = clear.parentElement
    expect(cluster).toBe(count.parentElement)
    expect(cluster?.contains(screen.getByRole('heading', { name: /^animes$/i }))).toBe(false)
    expect(cluster?.querySelector('input')).toBeNull()
  })

  it('restores search, unwatched and type from the hash', async () => {
    window.location.hash = '#/status?type=airing&unwatched=1&q=Airing'
    await renderStatus()
    expect((screen.getByPlaceholderText(/search animes|buscar animes/i) as HTMLInputElement).value).toBe('Airing')
    expect(screen.getByRole('button', { name: /unwatched|não vistos/i }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getAllByRole('radio', { name: /airing|saindo/i })[0].getAttribute('aria-checked')).toBe('true')
    expect(screen.queryByText('Finished Show')).toBeNull()
  })

  it('shows the empty filtered copy when a type has no rows', async () => {
    await renderStatus([
      anime({ anime_id: 1, name: 'Only Finished', media_status: 'FINISHED' }),
    ])
    await fireEvent.click(screen.getAllByRole('radio', { name: /airing|saindo/i })[0])
    await tick()
    expect(screen.getByText(/no animes match|nenhum anime corresponde/i)).toBeTruthy()
  })

  it('has Standalone as a type option and no Standalone-only chip', async () => {
    await renderStatus()
    expect(screen.getAllByRole('radio', { name: /standalone|avulsos/i }).length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: /standalone only|só avulsos/i })).toBeNull()
  })
})

describe('Status — user filter', () => {
  beforeEach(() => {
    window.location.hash = '#/status'
  })

  const acc = (source: 'anilist' | 'mal', username: string, watched: number, pending: number) => ({
    source,
    username,
    episodes_watched: watched,
    episodes_pending: pending,
  })
  // "Shared" is behind for bob (combined watched 2) but finished for alice.
  const shared = anime({
    anime_id: 1, name: 'Shared Show', episodes_released: 12, episodes_watched: 2, episodes_pending: 10,
    accounts: [acc('anilist', 'alice', 12, 0), acc('anilist', 'bob', 2, 10)],
  })
  const bobOnly = anime({
    anime_id: 2, name: 'Bob Show', episodes_released: 12, episodes_watched: 1, episodes_pending: 11,
    accounts: [acc('anilist', 'bob', 1, 11)],
  })
  const loose = anime({ anime_id: 3, name: 'Loose Show', is_standalone: true })
  const users = [shared, bobOnly, loose]

  it('does not render with a single account', async () => {
    await renderStatus(users, { anilist_usernames: ['alice'] })
    expect(screen.queryByRole('group', { name: /filter by user|filtrar por usuário/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /all users|todos os usuários/i })).toBeNull()
  })

  // The trigger shows the pick ("All users", "bob", "alice +1"); the menu stays open between picks.
  const openMenu = () => fireEvent.click(screen.getByRole('button', { name: /all users|todos os usuários/i }))
  const pick = async (name: RegExp) => {
    await fireEvent.click(screen.getByRole('menuitemcheckbox', { name }))
    await tick()
  }

  it('there are no per-user chips, only the multi-select, whatever the number of accounts', async () => {
    await renderStatus(users, { anilist_usernames: ['alice', 'bob'] })
    expect(screen.queryByRole('group', { name: /filter by user|filtrar por usuário/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /^alice / })).toBeNull()
    await openMenu()
    expect(screen.getAllByRole('menuitemcheckbox')).toHaveLength(2)
  })

  it('picking an account keeps only its animes and Unwatched answers for it', async () => {
    await renderStatus(users, { anilist_usernames: ['alice', 'bob'] })
    await openMenu()
    await pick(/^alice 1$/)
    expect(screen.getByRole('menuitemcheckbox', { name: /^alice / }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getAllByText('Shared Show').length).toBeGreaterThan(0)
    expect(screen.queryByText('Bob Show')).toBeNull()
    expect(screen.queryByText('Loose Show')).toBeNull()

    // Combined progress says "Shared" is unwatched (bob is at 2/12); alice finished it.
    await fireEvent.click(screen.getByRole('button', { name: /unwatched|não vistos/i }))
    await tick()
    expect(screen.queryByText('Shared Show')).toBeNull()
  })

  it('two picks are OR', async () => {
    await renderStatus(users, { anilist_usernames: ['alice', 'bob', 'carol'], mal_usernames: ['dave'] })
    await openMenu()
    await pick(/^alice /)
    await pick(/^bob /)
    expect(screen.queryByText('Loose Show')).toBeNull()
    expect(screen.getByText(/2 of 3|2 de 3/)).toBeTruthy()
  })

  it('labels the source only when the same username exists on both', async () => {
    await renderStatus(users, { anilist_usernames: ['alice', 'bob'], mal_usernames: ['bob'] })
    await openMenu()
    expect(screen.getByRole('menuitemcheckbox', { name: /^bob \(AniList\) / })).toBeTruthy()
    expect(screen.getByRole('menuitemcheckbox', { name: /^bob \(MAL\) / })).toBeTruthy()
    expect(screen.getByRole('menuitemcheckbox', { name: /^alice / })).toBeTruthy()
  })

  it('restores the accounts from the hash and ignores one no longer configured', async () => {
    window.location.hash = '#/status?account=anilist%3Abob&account=anilist%3Agone'
    await renderStatus(users, { anilist_usernames: ['alice', 'bob'] })
    expect(screen.getByRole('button', { name: /^bob$/ })).toBeTruthy()
    expect(screen.queryByText('Loose Show')).toBeNull()
    expect(screen.getByText(/2 of 3|2 de 3/)).toBeTruthy()
    // The gone account leaves the URL too, so the link no longer claims a filter it can't apply.
    await tick()
    expect(window.location.hash).not.toContain('gone')
  })

  it('keeps the URL selection when the config fails to load instead of showing everything', async () => {
    window.location.hash = '#/status?account=anilist%3Abob'
    await renderStatus(users, new Error('boom'))
    expect(screen.getAllByText('Bob Show').length).toBeGreaterThan(0)
    expect(screen.queryByText('Loose Show')).toBeNull()
    expect(window.location.hash).toContain('bob')
  })
})
