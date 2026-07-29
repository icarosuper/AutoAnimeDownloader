import { describe, it, expect } from 'vitest'
import { logSourceFromCaller } from '../../src/lib/domain/logSource'

describe('logSourceFromCaller', () => {
  it('maps a daemon-package file to scheduler', () => {
    expect(logSourceFromCaller('loop.go:42')).toBe('scheduler')
  })

  it('maps an api endpoint file to api', () => {
    expect(logSourceFromCaller('endpoint_torrents.go:229')).toBe('api')
  })

  it('maps a non-endpoint api file to api', () => {
    expect(logSourceFromCaller('server.go:10')).toBe('api')
  })

  it('maps the anilist package file to anilist', () => {
    expect(logSourceFromCaller('anilist.go:88')).toBe('anilist')
  })

  it('maps a nyaa package file to rss', () => {
    expect(logSourceFromCaller('nyaa_match.go:15')).toBe('rss')
  })

  it('maps a torrents package file to torrent', () => {
    expect(logSourceFromCaller('sessionmanager.go:301')).toBe('torrent')
  })

  it('falls back to "other" for files outside the five known packages', () => {
    expect(logSourceFromCaller('tray.go:5')).toBe('other')
  })

  it('falls back to "other" for a missing or empty caller', () => {
    expect(logSourceFromCaller(undefined)).toBe('other')
    expect(logSourceFromCaller('')).toBe('other')
  })

  it('strips a leading path before matching the basename', () => {
    expect(logSourceFromCaller('internal/torrents/session.go:12')).toBe('torrent')
  })
})
