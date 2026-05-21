import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { getStatus, getAnimes } from '../../src/lib/api/client.js'

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
})
