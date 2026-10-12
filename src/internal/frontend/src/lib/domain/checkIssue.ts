// The boundary is the same one lib/domain/animeState.ts documents and defends: the backend sends
// CODE + NUMBERS, the frontend builds the sentence. A ready string from Go would force the daemon
// to know the browser's locale.
import * as m from '../i18n/messages.js'
import type { Issue } from '../api/client.js'

export function issueMessage(issue: Issue): string {
  switch (issue.code) {
    case 'all_above_size_limit':
      return m.lastcheck_all_above_size_limit({
        candidates: issue.candidates ?? 0,
        limit: issue.limit_gb ?? 0,
      })
    case 'no_seeders':
      return m.lastcheck_no_seeders({
        candidates: issue.candidates ?? 0,
        seeders: issue.min_seeders ?? 0,
      })
    case 'no_torrent_found':
      return m.lastcheck_no_torrent_found()
    case 'disk_full':
      return m.lastcheck_disk_full()
    case 'torrent_rejected':
      return m.lastcheck_torrent_rejected({ candidates: issue.candidates ?? 0 })
    case 'awaiting_earlier_episode':
      return m.lastcheck_awaiting_earlier_episode()
    case 'max_episodes_per_anime':
      return m.lastcheck_max_episodes_per_anime({
        downloaded: issue.downloaded ?? 0,
        pending: issue.pending ?? 0,
      })
    default:
      // A new code in the backend can't become a blank line on the screen.
      return m.lastcheck_unknown({ code: issue.code })
  }
}

/** The explanation of why the batch didn't make it into that anime. "" when it was never eligible. */
export function batchNote(issue: Issue): string {
  switch (issue.batch_skipped) {
    case 'no_result':
      return m.lastcheck_batch_no_result()
    case 'above_size_limit':
      return m.lastcheck_batch_above_size_limit()
    case 'no_coverage':
      return m.lastcheck_batch_no_coverage()
    default:
      return ''
  }
}
