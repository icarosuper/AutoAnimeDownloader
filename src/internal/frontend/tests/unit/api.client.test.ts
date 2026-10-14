import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { getStatus, getAnimes, getTorrents, getAnimeDetail } from '../../src/lib/api/client.js'
import { toasts } from '../../src/lib/stores/toast.js'
import { locale } from '../../src/lib/stores/locale.js'

describe('api client', () => {
  const mockFetch = vi.fn()

  beforeEach(() => {
    // Fake timers so afterEach can expire every toast: the store drops a toast identical to one
    // still on screen, and the tests reuse messages.
    vi.useFakeTimers()
    vi.stubGlobal('fetch', mockFetch)
    mockFetch.mockReset()
  })

  afterEach(() => {
    vi.runAllTimers()
    vi.useRealTimers()
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

  it('getAnimeDetail 500 toasts the AniList i18n message, not a hardcoded string', async () => {
    locale.set('en')
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'Internal error' } }),
    })
    await expect(getAnimeDetail(1)).rejects.toThrow()
    const list = get(toasts)
    expect(list[list.length - 1].message).toBe('Failed to reach AniList')
  })

  it('getAnimeDetail 500 toasts in Portuguese when the locale is pt-BR', async () => {
    locale.set('pt-BR')
    try {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({ success: false, error: { code: 'ERR', message: 'Internal error' } }),
      })
      await expect(getAnimeDetail(1)).rejects.toThrow()
      const list = get(toasts)
      expect(list[list.length - 1].message).toBe('Falha na comunicação com o AniList')
    } finally {
      locale.set('en')
    }
  })

  it('non-Error rejection toasts the unknown-error i18n message', async () => {
    locale.set('en')
    mockFetch.mockRejectedValueOnce('boom')
    await expect(getStatus()).rejects.toBe('boom')
    const list = get(toasts)
    expect(list[list.length - 1].message).toBe('Unknown error')
  })

  it('a 5xx toast links to the report screen with the request as context', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'Internal error' } }),
    })
    await expect(getAnimeDetail(42)).rejects.toThrow()
    const href = get(toasts).at(-1)!.link!.href
    const params = new URLSearchParams(href.split('?')[1])
    expect(href.startsWith('#/report?')).toBe(true)
    expect(params.get('error')).toBe('Internal error')
    expect(params.get('context')).toBe('GET /animes/42/episodes 500')
    expect(params.get('anime')).toBe('42')
  })

  it('a 4xx toast has no report link', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: async () => ({ success: false, error: { code: 'ERR', message: 'Bad input' } }),
    })
    await expect(getAnimes()).rejects.toThrow('Bad input')
    const last = get(toasts).at(-1)!
    expect(last.message).toBe('Bad input')
    expect(last.link).toBeUndefined()
  })

  it('an aborted request toasts without a report link', async () => {
    const abort = new Error('aborted')
    abort.name = 'AbortError'
    mockFetch.mockRejectedValueOnce(abort)
    await expect(getAnimes()).rejects.toThrow('aborted')
    expect(get(toasts).at(-1)!.link).toBeUndefined()
  })
})
