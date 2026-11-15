import { describe, it, expect } from 'vitest'
import { sourceOptions, toggleSource, SOURCE_STANDALONE } from '../../src/lib/domain/webhookSources'

describe('sourceOptions', () => {
  it('lists the accounts, then standalone', () => {
    expect(sourceOptions(['alice', 'bob'], [])).toEqual([
      { key: 'anilist:alice', kind: 'account', username: 'alice' },
      { key: 'anilist:bob', kind: 'account', username: 'bob' },
      { key: SOURCE_STANDALONE, kind: 'standalone' },
    ])
  })

  it('names the service only when the same username is on both', () => {
    const opts = sourceOptions(['alice'], ['alice', 'carol'])
    expect(opts.slice(0, 3)).toEqual([
      { key: 'anilist:alice', kind: 'account', username: 'alice', service: 'AniList' },
      { key: 'mal:alice', kind: 'account', username: 'alice', service: 'MAL' },
      { key: 'mal:carol', kind: 'account', username: 'carol' },
    ])
  })

  it('keeps a selected key whose account is gone, so it can be unchecked', () => {
    expect(sourceOptions(['alice'], [], ['anilist:gone', 'anilist:alice']).at(-1)).toEqual({ key: 'anilist:gone', kind: 'stale' })
  })
})

describe('toggleSource', () => {
  it('adds and removes without duplicates', () => {
    expect(toggleSource([], 'anilist:a', true)).toEqual(['anilist:a'])
    expect(toggleSource(['anilist:a'], 'anilist:a', true)).toEqual(['anilist:a'])
    expect(toggleSource(['anilist:a', 'standalone'], 'anilist:a', false)).toEqual(['standalone'])
  })

  it('treats a missing list as empty', () => {
    expect(toggleSource(undefined, 'standalone', true)).toEqual(['standalone'])
  })
})
