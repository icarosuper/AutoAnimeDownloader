import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import Status from '../../src/routes/Status.svelte'
import * as client from '../../src/lib/api/client.js'
import type { CheckReport, StatusResponse } from '../../src/lib/api/client.js'

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

function statusResponse(overrides: Partial<StatusResponse> = {}): StatusResponse {
  return {
    status: 'running',
    last_check: '2026-08-19T12:00:00Z',
    has_error: false,
    version: '1.0.0',
    disk_total: 100,
    disk_free: 90,
    disk_low: false,
    ...overrides,
  }
}

const emptyReport: CheckReport = {
  finished_at: '0001-01-01T00:00:00Z',
  pass_error: '',
  problems: [],
  limits: [],
}

describe('Status — relatório da última verificação', () => {
  it('não renderiza nada quando o passe foi limpo', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse())
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue(emptyReport)

    render(Status)
    await tick()
    await tick()

    expect(screen.queryByTestId('last-check-report')).toBeNull()
  })

  it('lista problemas e limites quando existem', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse())
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z',
      pass_error: '',
      problems: [{ anime_id: 269, anime_name: 'Bleach', episodes: [12, 15], code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }],
      limits: [{ anime_id: 21, anime_name: 'One Piece', code: 'max_episodes_per_anime', downloaded: 12, pending: 35, batch_skipped: 'no_result' }],
    })

    render(Status)
    await tick()
    await tick()

    const card = await screen.findByTestId('last-check-report')
    expect(card.textContent).toContain('Bleach')
    expect(card.textContent).toContain('One Piece')
    expect(card.textContent).toContain('8')
    expect(card.textContent).toContain('35')
  })

  it('mostra a mensagem do erro de passe no lugar do texto genérico', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse({ has_error: true }))
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({ ...emptyReport, pass_error: 'anilist unreachable' })

    render(Status)
    await tick()
    await tick()

    expect(await screen.findByText(/anilist unreachable/)).toBeTruthy()
  })
})
