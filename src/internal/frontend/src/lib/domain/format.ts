/**
 * `format.ts` (spec §7, Fase 2) — locale-sensitive formatting for speed, bytes, percent, ETA
 * and date. pt-BR uses a comma decimal separator and a dot thousands separator ("4,2 MB/s",
 * "1.023,9 KB"); en uses the reverse. Every function takes `locale` explicitly instead of
 * reading `lib/stores/locale.ts` — that store is a Svelte `writable`, and pulling it in here
 * would be exactly the dependency `lib/domain/` is not allowed to have. Screens read
 * `$locale` and pass it through.
 *
 * "—" (not a translated string) is the shared placeholder for "no value" across every
 * function here, matching the existing convention in `lib/utils/torrents.ts` (its `IDLE`
 * constant) — a plain dash needs no locale and keeps this module string-free beyond units.
 *
 * Overlap with existing helpers (documented in the Fase 2 report, not resolved here per the
 * brief — resolving it means editing routes/*.svelte, out of scope this phase):
 *   - `formatSpeed`/`formatPercent`/`formatEta` overlap `lib/utils/torrents.ts`'s
 *     same-named exports (those are locale-agnostic; Downloads.svelte/AnimeDetail.svelte
 *     still import the old ones).
 *   - `formatBytes` overlaps `lib/utils/status.ts`'s `formatBytes` (that one is a fixed-GB
 *     disk-space formatter, not auto-unit; Status.svelte still imports the old one).
 *   - `formatDate` overlaps the local (unexported) `formatDate` in AnimeDetail.svelte, which
 *     just calls `.toLocaleString()` inline.
 * Consolidating each call site belongs to the screen phase that touches that file (Fase 3
 * Status, Fase 4 AnimeDetail, Fase 5 Downloads).
 */

export type FormatLocale = 'en' | 'pt-BR'

const IDLE = '—'
const SPEED_UNITS = ['KB/s', 'MB/s', 'GB/s']
const BYTE_UNITS = ['KB', 'MB', 'GB', 'TB']

function formatDecimal(value: number, locale: FormatLocale): string {
  return new Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(value)
}

function formatInteger(value: number, locale: FormatLocale): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 0 }).format(value)
}

/** Steps a byte count up through `units` while it's >= 1024, one unit per step. */
function stepUnits(amount: number, units: string[]): { value: number; unit: string | null } {
  let value = amount
  let index = -1
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index++
  }
  return { value, unit: index === -1 ? null : units[index] }
}

/**
 * Same rate as `formatSpeed`, but with the number and its unit kept apart. The Status hero
 * (spec §9.1) sets them at two different sizes/weights — 46px/600 for the number, small
 * secondary text for "MB/s" — which can't be done with one pre-joined string. Splitting the
 * joined string back apart at the screen would mean parsing a locale-formatted number, so the
 * split happens here, at the point where the two halves are still separate values, and
 * `formatSpeed` re-joins them for every caller that just wants the sentence form.
 *
 * An idle rate has no unit to show: `value` is the dash and `unit` is empty.
 */
export function formatSpeedParts(
  bytesPerSecond: number,
  locale: FormatLocale,
): { value: string; unit: string } {
  if (!bytesPerSecond || bytesPerSecond <= 0) return { value: IDLE, unit: '' }
  const { value, unit } = stepUnits(bytesPerSecond, SPEED_UNITS)
  if (unit === null) return { value: formatInteger(value, locale), unit: 'B/s' }
  return { value: formatDecimal(value, locale), unit }
}

/** Formats a byte-per-second rate. Zero renders as a dash: an idle torrent has no speed. */
export function formatSpeed(bytesPerSecond: number, locale: FormatLocale): string {
  const { value, unit } = formatSpeedParts(bytesPerSecond, locale)
  return unit ? `${value} ${unit}` : value
}

/** Formats a byte count with an auto-selected unit (B through TB). */
export function formatBytes(bytes: number, locale: FormatLocale): string {
  const { value, unit } = stepUnits(bytes, BYTE_UNITS)
  if (unit === null) return `${formatInteger(value, locale)} B`
  return `${formatDecimal(value, locale)} ${unit}`
}

/** Formats a 0..1 progress fraction as a whole, clamped percentage. */
export function formatPercent(fraction: number, locale: FormatLocale): string {
  const clamped = Math.max(0, Math.min(1, fraction || 0))
  return `${formatInteger(Math.round(clamped * 100), locale)}%`
}

/**
 * Formats a remaining time in seconds. null means infinite/unknown — rain only fills ETA
 * while the torrent is downloading at a non-zero speed, so null is the common case.
 */
export function formatEta(seconds: number | null, locale: FormatLocale): string {
  if (seconds === null || seconds === undefined || seconds < 0) return IDLE
  if (seconds < 60) return `${formatInteger(seconds, locale)}s`
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60)
    return `${formatInteger(minutes, locale)}m ${formatInteger(seconds % 60, locale)}s`
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600)
    return `${formatInteger(hours, locale)}h ${formatInteger(Math.floor((seconds % 3600) / 60), locale)}m`
  }
  const days = Math.floor(seconds / 86400)
  return `${formatInteger(days, locale)}d ${formatInteger(Math.floor((seconds % 86400) / 3600), locale)}h`
}

/**
 * Formats an ISO date string (or any `Date`-constructor-accepted value) in the runtime's local
 * timezone, locale-ordered (dd/mm/yyyy for pt-BR, m/d/yy for en). Falsy input, or a value that
 * doesn't parse to a valid date, renders as a dash.
 */
export function formatDate(value: string | number | Date | null | undefined, locale: FormatLocale): string {
  if (!value) return IDLE
  const date = value instanceof Date ? value : new Date(value)
  if (isNaN(date.getTime())) return IDLE
  return new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(date)
}
