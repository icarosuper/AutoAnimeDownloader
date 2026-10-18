/**
 * `parseLogLine` (Phase 7, spec §9.5) — turns a raw `GET /logs` line into the four
 * columns of the `82px 60px 90px 1fr` grid: time, level, source, message.
 *
 * This function used to live inside `Logs.svelte`. It moved to `lib/domain/` (no Svelte
 * dependency, covered by vitest, like the other §7 modules) for a concrete reason, not
 * aesthetics: the SOURCE column is derived from zerolog's `caller` field via `logSource.ts`,
 * and the old parser buried `caller` inside `extras` with the rest of the structured
 * fields. Extracting `caller` as its own field is what makes the column possible.
 *
 * The three accepted input formats are the ones the daemon actually produces, tried in
 * this order: zerolog JSON (the normal case), zerolog console (`INF`/`WRN`/... — the
 * dev format, when output is a TTY), and anything else (keyword fallback, which never
 * fails: a line with no recognizable level becomes "info").
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type ParsedLogLine = {
  level: LogLevel
  /** Time already formatted as HH:MM:SS; empty string when the line has no `time`. */
  time: string
  message: string
  /** The raw line, which is what the copy button delivers. */
  raw: string
  /** Remaining structured fields, rendered faded after the message. */
  extras?: string
  /** `<file>.go:<line>` — feeds `logSourceFromCaller`. */
  caller?: string
}

const CONSOLE_LEVELS: Record<string, LogLevel> = {
  DBG: 'debug',
  INF: 'info',
  WRN: 'warn',
  // zerolog treats FTL as a fatal error; for anyone reading the screen it's an error.
  ERR: 'error',
  FAT: 'error',
}

const KNOWN_LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error']

function normalizeLevel(value: unknown): LogLevel {
  const text = String(value ?? '').toLowerCase()
  if ((KNOWN_LEVELS as string[]).includes(text)) return text as LogLevel
  // zerolog emits "fatal"/"panic" in JSON; both are errors for anyone reading.
  if (text === 'fatal' || text === 'panic') return 'error'
  return 'info'
}

/**
 * `HH:MM:SS` in the local timezone. The column is 82px in mono — the date wouldn't fit,
 * and it's redundant on a screen that shows the last N lines. A `time` that isn't a
 * valid date is returned raw, instead of becoming "Invalid Date".
 */
export function formatLogTime(timestamp: string | undefined): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) return timestamp
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

export function parseLogLine(line: string): ParsedLogLine {
  if (line.startsWith('{')) {
    try {
      const json = JSON.parse(line)
      // `caller` is pulled out of `rest` on purpose: it becomes the source column, and
      // repeating it at the end of the message would be the same information twice on one line.
      const { level, time, message, caller, ...rest } = json
      const extrasString =
        Object.keys(rest).length > 0
          ? Object.entries(rest)
              .map(([k, v]) => `"${k}"="${typeof v === 'object' ? JSON.stringify(v) : String(v)}"`)
              .join(' ')
          : undefined
      return {
        level: normalizeLevel(level),
        time: formatLogTime(time),
        message: message || line,
        raw: line,
        extras: extrasString,
        caller: caller ? String(caller) : undefined,
      }
    } catch {
      /* Malformed JSON falls through to the heuristics below */
    }
  }

  const levelMatch = line.match(/\b(DBG|INF|WRN|ERR|FAT)\b/)
  if (levelMatch) {
    return { level: CONSOLE_LEVELS[levelMatch[1]] ?? 'info', time: '', message: line, raw: line }
  }

  const lower = line.toLowerCase()
  const level: LogLevel = lower.includes('error') || lower.includes('err')
    ? 'error'
    : lower.includes('warn')
      ? 'warn'
      : lower.includes('debug') || lower.includes('dbg')
        ? 'debug'
        : 'info'
  return { level, time: '', message: line, raw: line }
}

/** Count by level for the §9.5 filter pills — always over the list already fetched. */
export function countByLevel(parsed: ParsedLogLine[]): Record<LogLevel | 'all', number> {
  const counts = { all: parsed.length, debug: 0, info: 0, warn: 0, error: 0 }
  for (const entry of parsed) counts[entry.level]++
  return counts
}
