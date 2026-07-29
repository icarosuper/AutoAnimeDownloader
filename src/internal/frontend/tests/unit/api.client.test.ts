import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { getStatus, getAnimes, getTorrents } from '../../src/lib/api/client.js'
import { toasts } from '../../src/lib/stores/toasts.js'

describe('api client', () => {
  const mockFetch = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch)
    mockFetch.mockReset()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('getStatus returns parsed data on success', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { status: 'running', last_check: '2026-01-01T00:00:00Z', has_error: false, version: '1.0' },
      }),
    })
    const result = await getStatus()
    expect(result.status).toBe('running')
    expect(result.has_error).toBe(false)
    expect(mockFetch).toHaveBeenCalledWith(
      expect.stringContaining('/status'),
      expect.objectContaining({ method: 'GET' }),
    )
  })

  it('getStatus rejects with server error message on HTTP failure', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'Internal error' } }),
    })
    await expect(getStatus()).rejects.toThrow('Internal error')
  })

  it('getAnimes returns empty array', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ success: true, data: [] }),
    })
    const result = await getAnimes()
    expect(result).toEqual([])
  })

  it('getStatus failure still pushes an error toast (non-polling endpoints unaffected)', async () => {
    const before = get(toasts).length
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'Internal error' } }),
    })
    await expect(getStatus()).rejects.toThrow('Internal error')
    expect(get(toasts).length).toBe(before + 1)
  })

  it('getTorrents failure does NOT push an error toast (silent poll, still rejects)', async () => {
    const before = get(toasts).length
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'torrents down' } }),
    })
    await expect(getTorrents()).rejects.toThrow('torrents down')
    expect(get(toasts).length).toBe(before)
  })
})
