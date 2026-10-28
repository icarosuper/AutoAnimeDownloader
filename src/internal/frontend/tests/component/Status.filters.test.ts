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

async function renderStatus(animes: AnimeInfo[] = library) {
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
  vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
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
