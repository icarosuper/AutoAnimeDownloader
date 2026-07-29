/**
 * `parseLogLine` (Fase 7, spec §9.5) — transforma uma linha crua de `GET /logs` nas quatro
 * colunas do grid `82px 60px 90px 1fr`: horário, nível, origem, mensagem.
 *
 * A função vivia dentro de `Logs.svelte`. Ela subiu para `lib/domain/` (sem dependência de
 * Svelte, coberta por vitest, como os outros módulos do §7) por uma razão concreta e não
 * estética: a coluna de ORIGEM é derivada do campo `caller` do zerolog via `logSource.ts`, e
 * o parser antigo enterrava `caller` no meio de `extras` junto com o resto dos campos
 * estruturados. Extrair `caller` como campo próprio é o que torna a coluna possível.
 *
 * Os três formatos de entrada aceitos são os que o daemon realmente produz, na ordem em que
 * são tentados: JSON do zerolog (o normal), console do zerolog (`INF`/`WRN`/... — o formato
 * de dev, quando o output é um TTY), e qualquer outra coisa (fallback por palavra-chave, que
 * nunca falha: uma linha sem nível reconhecível vira "info").
 */

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type ParsedLogLine = {
  level: LogLevel
  /** Horário já formatado como HH:MM:SS; string vazia quando a linha não tem `time`. */
  time: string
  message: string
  /** A linha crua, que é o que o botão de copiar entrega. */
  raw: string
  /** Campos estruturados restantes, renderizados esmaecidos após a mensagem. */
  extras?: string
  /** `<arquivo>.go:<linha>` — alimenta `logSourceFromCaller`. */
  caller?: string
}

const CONSOLE_LEVELS: Record<string, LogLevel> = {
  DBG: 'debug',
  INF: 'info',
  WRN: 'warn',
  // zerolog trata FTL como um erro fatal; para quem lê a tela é um erro.
  ERR: 'error',
  FAT: 'error',
}

const KNOWN_LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error']

function normalizeLevel(value: unknown): LogLevel {
  const text = String(value ?? '').toLowerCase()
  if ((KNOWN_LEVELS as string[]).includes(text)) return text as LogLevel
  // zerolog emite "fatal"/"panic" em JSON; ambos são erro para quem lê.
  if (text === 'fatal' || text === 'panic') return 'error'
  return 'info'
}

/**
 * `HH:MM:SS` no fuso local. A coluna tem 82px em mono — a data não caberia, e é redundante
 * numa tela que mostra as últimas N linhas. Um `time` que não é uma data válida volta cru,
 * em vez de virar "Invalid Date".
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
      // `caller` sai de `rest` de propósito: ele vira a coluna de origem, e repeti-lo no fim
      // da mensagem seria a mesma informação duas vezes na mesma linha.
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
      /* JSON malformado cai nos heurísticos abaixo */
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

/** Contagem por nível para as pills de filtro do §9.5 — sempre sobre a lista JÁ buscada. */
export function countByLevel(parsed: ParsedLogLine[]): Record<LogLevel | 'all', number> {
  const counts = { all: parsed.length, debug: 0, info: 0, warn: 0, error: 0 }
  for (const entry of parsed) counts[entry.level]++
  return counts
}
