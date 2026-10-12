import * as m from '../i18n/messages.js'

/**
 * The aborted-pass banner sentence, built from the CODE — never from Go's raw text. The same
 * boundary checkIssue.ts documents and defends. Before this the banner showed `err.Error()`
 * directly, which in practice dumped AniList's response JSON on the screen.
 */
export function passErrorMessage(code: string): string {
  switch (code) {
    case 'anilist':
      return m.passerror_anilist()
    case 'mal':
      return m.passerror_mal()
    case 'setup':
      return m.passerror_setup()
    case 'library':
      return m.passerror_library()
    case 'torrent_backend':
      return m.passerror_torrent_backend()
    case 'config':
      return m.passerror_config()
    case 'storage':
      return m.passerror_storage()
    default:
      // Covers 'unknown' and a new code the backend starts sending before the frontend knows
      // it: a check that didn't run can't become a blank line on the screen.
      return m.passerror_unknown()
  }
}
