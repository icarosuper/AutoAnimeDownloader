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

describe('Status — próximo episódio na lista', () => {
  // A lista renderiza duas vezes (grid do desktop e cards do mobile); no jsdom as duas saem no
  // DOM porque `lg:hidden` é só CSS. Cada asserção abaixo mira uma delas de propósito.
  it('preenche a coluna do desktop com o tempo cru, sem repetir o rótulo da coluna', async () => {
    const inThreeDays = Math.floor(Date.now() / 1000) + 3 * 86400 + 60
    await renderWith([anime({ next_airing_at: inThreeDays })])

    // Cabeçalho da coluna nova.
    expect(screen.getByText(/^Next ep\.$|^Próximo ep\.$/)).toBeTruthy()
    // Célula: só o tempo, já que o rótulo está no cabeçalho.
    expect(screen.getByText(/^in 3 days$|^em 3 dias$/)).toBeTruthy()
  })

  it('no card do mobile a frase vem inteira, junto do último download', async () => {
    const inThreeDays = Math.floor(Date.now() / 1000) + 3 * 86400 + 60
    await renderWith([anime({ next_airing_at: inThreeDays })])

    expect(screen.getByText(/Next ep\. in 3 days|Próx\. ep\. em 3 dias/)).toBeTruthy()
  })

  it('sem episódio agendado a coluna fica vazia e nada aparece no card', async () => {
    await renderWith([anime()])

    // O cabeçalho da coluna continua lá — é coluna, não badge. O que não pode aparecer é
    // valor nenhum: nem a frase do card mobile, nem o tempo na célula do desktop.
    expect(screen.queryByText(/Next ep\. \S|Próx\. ep\. \S/)).toBeNull()
    expect(screen.queryByText(/in \d+ days|em \d+ dias/)).toBeNull()
  })
})
