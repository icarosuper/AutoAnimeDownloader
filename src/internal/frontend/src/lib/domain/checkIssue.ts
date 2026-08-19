// A fronteira é a mesma que lib/domain/animeState.ts documenta e defende: o backend manda
// CÓDIGO + NÚMEROS, o frontend monta a frase. Uma string pronta vinda do Go obrigaria o daemon
// a saber o locale do navegador.
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
    case 'max_episodes_per_anime':
      return m.lastcheck_max_episodes_per_anime({
        downloaded: issue.downloaded ?? 0,
        pending: issue.pending ?? 0,
      })
    default:
      // Um código novo no backend não pode virar linha em branco na tela.
      return m.lastcheck_unknown({ code: issue.code })
  }
}

/** A explicação de por que o batch não entrou naquele anime. "" quando ele nunca foi elegível. */
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
