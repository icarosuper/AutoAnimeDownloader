import * as m from '../i18n/messages.js'

/**
 * A frase do banner de passe abortado, montada a partir do CÓDIGO — nunca do texto cru do Go.
 * Mesma fronteira que checkIssue.ts documenta e defende. Antes disso o banner exibia
 * `err.Error()` direto, o que na prática despejava o JSON de resposta da AniList na tela.
 */
export function passErrorMessage(code: string): string {
  switch (code) {
    case 'anilist':
      return m.passerror_anilist()
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
      // Inclui 'unknown' e um código novo que o backend passe a mandar antes de o frontend
      // conhecer: uma verificação que não rodou não pode virar linha em branco na tela.
      return m.passerror_unknown()
  }
}
