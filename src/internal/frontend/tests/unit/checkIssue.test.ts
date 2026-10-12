import { describe, it, expect } from 'vitest'
import { issueMessage, batchNote } from '../../src/lib/domain/checkIssue.js'
import type { Issue } from '../../src/lib/api/client.js'

function issue(overrides: Partial<Issue> = {}): Issue {
  return { anime_id: 1, anime_name: 'Bleach', code: 'no_torrent_found', ...overrides }
}

describe('issueMessage', () => {
  it('interpolates candidates and ceiling in the size problem', () => {
    const text = issueMessage(issue({ code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }))
    expect(text).toContain('8')
    expect(text).toContain('3')
  })

  it('interpolates candidates and seeders floor', () => {
    const text = issueMessage(issue({ code: 'no_seeders', candidates: 4, min_seeders: 5 }))
    expect(text).toContain('4')
    expect(text).toContain('5')
  })

  it('interpolates downloaded and pending in the per-anime limit', () => {
    const text = issueMessage(issue({ code: 'max_episodes_per_anime', downloaded: 12, pending: 35 }))
    expect(text).toContain('12')
    expect(text).toContain('35')
  })

  it('has a sentence for every known code', () => {
    const codes = [
      'all_above_size_limit',
      'no_seeders',
      'no_torrent_found',
      'disk_full',
      'torrent_rejected',
      'awaiting_earlier_episode',
      'max_episodes_per_anime',
    ]
    for (const code of codes) {
      expect(issueMessage(issue({ code })), code).not.toBe('')
    }
  })

  // A new code in the backend can't become a blank line on the screen: the fallback is the code
  // itself, ugly and readable, instead of silence.
  it('falls back to the raw code when it has no sentence for it', () => {
    expect(issueMessage(issue({ code: 'code_from_the_future' }))).toContain('code_from_the_future')
  })
})

describe('batchNote', () => {
  it('explains every reason for a skipped batch', () => {
    for (const value of ['no_result', 'above_size_limit', 'no_coverage']) {
      expect(batchNote(issue({ code: 'max_episodes_per_anime', batch_skipped: value })), value).not.toBe('')
    }
  })

  it('is empty when the anime was never eligible for a batch', () => {
    expect(batchNote(issue({ code: 'max_episodes_per_anime' }))).toBe('')
  })
})
