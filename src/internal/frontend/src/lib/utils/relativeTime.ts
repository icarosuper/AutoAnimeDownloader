/**
 * `formatRelative` serve passado e futuro com a mesma conta: `Intl.RelativeTimeFormat` só olha
 * o sinal, então "há 3 horas" e "em 3 horas" saem daqui.
 *
 * `Math.trunc` (e não `round`) é o corte para zero que o "há X" da tela de status já tinha:
 * 25h continuam sendo "há 1 dia", não "há 2 dias".
 *
 * Fica fora dos componentes por ser puro e testável sem Svelte — o `locale` entra como
 * argumento em vez de ser lido do store pelo mesmo motivo.
 */
export function formatRelative(targetMs: number, nowMs: number, locale: string): string {
  const diffSeconds = Math.trunc((targetMs - nowMs) / 1000)
  const abs = Math.abs(diffSeconds)
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  if (abs < 60) return rtf.format(diffSeconds, 'second')
  if (abs < 3600) return rtf.format(Math.trunc(diffSeconds / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.trunc(diffSeconds / 3600), 'hour')
  return rtf.format(Math.trunc(diffSeconds / 86400), 'day')
}

/**
 * `nextAiringIn` traduz o `next_airing_at` da API (unix em SEGUNDOS) para o texto relativo, ou
 * string vazia quando não há nada a mostrar. Duas razões para dar vazio:
 *
 * - `next_airing_at` ausente/0 — anime terminado ou em hiato entre temporadas; a AniList devolve
 *   `nextAiringEpisode: null` nos dois casos.
 * - horário já no passado — a AniList às vezes demora a publicar o episódio seguinte, e um
 *   "próximo episódio há 2 dias" seria mentira na tela.
 *
 * Devolve o texto do TEMPO, não a frase inteira: quem renderiza interpola em `m.next_airing`
 * (mesma fronteira de i18n de `domain/animeState.ts`).
 */
export function nextAiringIn(
  nextAiringAt: number | undefined,
  nowMs: number,
  locale: string,
): string {
  if (!nextAiringAt || nextAiringAt * 1000 <= nowMs) return ''
  return formatRelative(nextAiringAt * 1000, nowMs, locale)
}
