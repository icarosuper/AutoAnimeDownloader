import type { BackendState } from '../stores/backendHealth.js'
import type { AnilistHealth } from '../api/client.js'

/**
 * O banner é ÚNICO e tem precedência — nunca uma pilha. Com o daemon fora do ar, o estado da
 * AniList na tela é a última leitura bem-sucedida, ou seja, informação datada; mostrá-la ao lado
 * de "daemon não responde" é ruído contraditório. Ver decisions.md #66.
 *
 * A ordem vai do mais a montante para o mais a jusante: nada abaixo do backend é observável sem
 * o backend.
 */
export type BannerKind =
  | 'backend_unreachable'
  | 'backend_error'
  | 'anilist_outage'
  | 'anilist_rate_limited'
  | 'anilist_app_bug'

export interface SystemBanner {
  kind: BannerKind
  /** Mensagem crua da AniList, quando houver. Um 403 de IP bloqueado explica o motivo por
   *  escrito, e é a única informação que o frontend não tem como reconstruir sozinho. */
  detail?: string
  /** Instante em que o rate limit expira, para a contagem regressiva. */
  retryAt?: Date
  /** Botão de reportar SÓ em bug nosso: 5xx do backend, e 400 da AniList (o schema mudou e
   *  nenhuma espera resolve). Reportar rate limit ou outage gera issue que se fecha com "é a
   *  AniList", e treina o usuário a ignorar o botão justamente quando ele importa. */
  reportable: boolean
}

export function pickBanner(
  backend: BackendState,
  anilist: AnilistHealth | null | undefined,
): SystemBanner | null {
  if (backend === 'unreachable') return { kind: 'backend_unreachable', reportable: false }
  if (backend === 'server_error') return { kind: 'backend_error', reportable: true }

  switch (anilist?.state) {
    case 'rate_limited':
      return {
        kind: 'anilist_rate_limited',
        retryAt: anilist.retry_at ? new Date(anilist.retry_at) : undefined,
        reportable: false,
      }
    case 'outage':
      return { kind: 'anilist_outage', detail: anilist.message, reportable: false }
    case 'app_bug':
      return { kind: 'anilist_app_bug', detail: anilist.message, reportable: true }
    default:
      return null
  }
}

/** Segundos restantes até `retryAt`, nunca negativo. Fora do componente para ser testável. */
export function secondsUntil(retryAt: Date | undefined, now: number): number {
  if (!retryAt) return 0
  return Math.max(0, Math.ceil((retryAt.getTime() - now) / 1000))
}
