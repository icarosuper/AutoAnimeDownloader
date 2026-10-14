import { describe, it, expect } from 'vitest'
import {
  animeIdFromEndpoint,
  crashReaction,
  isUnexpectedFailure,
  issueUrl,
  reportFileName,
  reportHref,
  REPO_URL,
} from '../../src/lib/report.js'

describe('report helpers', () => {
  it('reportHref skips empty params', () => {
    expect(reportHref()).toBe('#/report')
    expect(reportHref({ error: 'boom', anime: undefined })).toBe('#/report?error=boom')
  })

  it('only 5xx, network failures and unreadable responses are unexpected', () => {
    expect(isUnexpectedFailure(500, new Error('x'))).toBe(true)
    expect(isUnexpectedFailure(0, new Error('x'))).toBe(true)
    expect(isUnexpectedFailure(200, new SyntaxError('bad json'))).toBe(true)
    expect(isUnexpectedFailure(404, new Error('x'))).toBe(false)
    const abort = new Error('aborted')
    abort.name = 'AbortError'
    expect(isUnexpectedFailure(0, abort)).toBe(false)
  })

  it('animeIdFromEndpoint reads the media id of anime endpoints only', () => {
    expect(animeIdFromEndpoint('/animes/42/episodes')).toBe('42')
    expect(animeIdFromEndpoint('/animes/42')).toBe('42')
    expect(animeIdFromEndpoint('/animes')).toBeUndefined()
    expect(animeIdFromEndpoint('/torrents/abc')).toBeUndefined()
  })

  it('issueUrl pre-fills the form fields by id and stays short', () => {
    const url = new URL(issueUrl('It broke\nmore detail' + 'x'.repeat(20000), 'Version: 2.5.0'))
    expect(url.origin + url.pathname).toBe(`${REPO_URL}/issues/new`)
    expect(url.searchParams.get('template')).toBe('bug_report.yml')
    expect(url.searchParams.get('title')).toBe('It broke')
    expect(url.searchParams.get('environment')).toBe('Version: 2.5.0')
    expect(url.searchParams.get('what-happened')).toContain('[truncated')
    expect(url.toString().length).toBeLessThan(6000)
  })

  it('reportFileName carries the version and the minute', () => {
    expect(reportFileName('2.5.0', new Date(2026, 9, 14, 9, 5))).toBe('aad-report-2.5.0-20261014-0905.txt')
  })

  it('crashReaction opens on a fresh load, toasts in an open tab, and reacts once', () => {
    expect(crashReaction('2026-10-14T10:00:00Z', null, true)).toBe('open')
    expect(crashReaction('2026-10-14T10:00:00Z', null, false)).toBe('toast')
    expect(crashReaction('2026-10-14T10:00:00Z', '2026-10-14T10:00:00Z', true)).toBeNull()
    expect(crashReaction(null, null, true)).toBeNull()
    expect(crashReaction(undefined, null, true)).toBeNull()
  })
})
