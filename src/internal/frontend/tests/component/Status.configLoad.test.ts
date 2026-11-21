import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/svelte'
import { tick } from 'svelte'
import Status from '../../src/routes/Status.svelte'
import * as client from '../../src/lib/api/client.js'
import type { StatusResponse } from '../../src/lib/api/client.js'

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

type StatusListener = (status: string, lastCheck: string, hasError: boolean) => void
const listeners: StatusListener[] = []
vi.mock('../../src/lib/websocket/shared.js', () => ({
  ensureWsConnected: () => {},
  onDaemonStatus: (fn: StatusListener) => {
    listeners.push(fn)
    return () => {}
  },
}))

describe('Status — config load', () => {
  // The mount load and the WebSocket's "running" (which reloads the animes, and the config while
  // it hasn't loaded) both asked before the first answer; two in flight could land out of order
  // and leave the older config on screen.
  it('asks for the config once while a request is still in flight', async () => {
    vi.mocked(client.getStatus).mockResolvedValue({
      status: 'running', last_check: '', has_error: false, version: '1.0.0', disk_total: 100, disk_free: 90, disk_low: false,
    } as StatusResponse)
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockReturnValue(new Promise(() => {}) as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({ finished_at: '0001-01-01T00:00:00Z', pass_error: '', problems: [], limits: [] })

    render(Status)
    await tick()
    expect(listeners.length).toBeGreaterThan(0)
    for (const fn of listeners) fn('running', '', false)
    await tick()

    expect(client.getAnimes).toHaveBeenCalledTimes(2)
    expect(client.getConfig).toHaveBeenCalledTimes(1)
  })
})
