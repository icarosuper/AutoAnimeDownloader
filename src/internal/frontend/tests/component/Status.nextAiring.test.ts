import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
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
    anime_id: 42,
    name: 'Vivy',
    episodes_downloaded: 3,
    episodes_released: 3,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 13,
    latest_episode_number: 3,
    last_download_date: '2026-08-19T12:00:00Z',
    ...overrides,
  }
}

async function renderWith(animes: AnimeInfo[]) {
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
  render(Status)
  await tick()
  await tick()
  await tick()
}

describe('Status — next episode in the list', () => {
  // The list renders twice (desktop grid and mobile cards); in jsdom both land in the DOM
  // because `lg:hidden` is CSS only. Each assertion below aims at one of them on purpose.
  it('fills the desktop column with the raw time, without repeating the column label', async () => {
    const inThreeDays = Math.floor(Date.now() / 1000) + 3 * 86400 + 60
    await renderWith([anime({ next_airing_at: inThreeDays })])

    // New column header.
    expect(screen.getByText(/^Next ep\.$|^Próximo ep\.$/)).toBeTruthy()
    // Cell: just the time, since the label is in the header.
    expect(screen.getByText(/^in 3 days$|^em 3 dias$/)).toBeTruthy()
  })

  it('on the mobile card the whole phrase comes together, next to the last download', async () => {
    const inThreeDays = Math.floor(Date.now() / 1000) + 3 * 86400 + 60
    await renderWith([anime({ next_airing_at: inThreeDays })])

    expect(screen.getByText(/Next ep\. in 3 days|Próx\. ep\. em 3 dias/)).toBeTruthy()
  })

  it('with no scheduled episode the column stays empty and nothing appears on the card', async () => {
    await renderWith([anime()])

    // The column header is still there — it's a column, not a badge. What must not appear is
    // any value: neither the mobile-card phrase, nor the time in the desktop cell.
    expect(screen.queryByText(/Next ep\. \S|Próx\. ep\. \S/)).toBeNull()
    expect(screen.queryByText(/in \d+ days|em \d+ dias/)).toBeNull()
  })
})
