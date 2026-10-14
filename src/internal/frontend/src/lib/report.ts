/**
 * Bug report helpers: how the app links to the report screen and to GitHub. Nothing here sends
 * anything — the user attaches the file by hand (decisions.md #102).
 */

/** The ONE place the repo URL lives: the v3.0.0 rename changes it. */
export const REPO_URL = 'https://github.com/icarosuper/AutoAnimeDownloader'

/** What fits in the issue URL before GitHub cuts it off; the full text is in the file anyway. */
const URL_TEXT_BUDGET = 4000

const LAST_CRASH_KEY = 'aad.lastPendingCrash'

export interface ReportContext {
  error?: string
  context?: string
  anime?: string
}

/** Hash route of the report screen, pre-filled with what the app caught. */
export function reportHref(ctx: ReportContext = {}): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(ctx)) {
    if (value) params.set(key, value)
  }
  const qs = params.toString()
  return qs ? `#/report?${qs}` : '#/report'
}

/**
 * An API failure worth reporting: the app broke (5xx, network down, a response it couldn't read).
 * A 4xx is the app answering correctly ("username not found"), and an abort is the screen's own
 * doing — a Report link on either only produces junk issues.
 */
export function isUnexpectedFailure(status: number, error: unknown): boolean {
  if (error instanceof Error && error.name === 'AbortError') return false
  return status < 400 || status >= 500
}

/** The media id when the failing endpoint is about one anime (/animes/{id}/...). */
export function animeIdFromEndpoint(endpoint: string): string | undefined {
  return /^\/animes\/(\d+)(?:\/|$)/.exec(endpoint)?.[1]
}

function truncateForUrl(text: string, budget: number): string {
  let out = text
  while (out && encodeURIComponent(out).length > budget) {
    out = out.slice(0, Math.floor(out.length * 0.9))
  }
  return out === text ? out : `${out}\n\n[truncated — the full text is in the attached file]`
}

/** The issue form, pre-filled by field id (see .github/ISSUE_TEMPLATE/bug_report.yml). */
export function issueUrl(whatHappened: string, environment: string): string {
  const firstLine = whatHappened.trim().split('\n')[0].slice(0, 80)
  const params = new URLSearchParams({
    template: 'bug_report.yml',
    title: firstLine,
    'what-happened': truncateForUrl(whatHappened.trim(), URL_TEXT_BUDGET),
    environment,
  })
  return `${REPO_URL}/issues/new?${params.toString()}`
}

/** aad-report-<version>-<yyyymmdd-hhmm>.txt */
export function reportFileName(version: string, now: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
  return `aad-report-${version}-${stamp}.txt`
}

export function reportFileContent(whatHappened: string, bundle: string): string {
  return `## What happened\n\n${whatHappened.trim()}\n\n${bundle}`
}

/** Saves the text as a file, synchronously — "Open GitHub issue" calls window.open right after. */
export function downloadText(fileName: string, text: string): void {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

/**
 * What to do about `pending_crash_at` from /status. A fresh page load opens the report screen; a tab
 * that was already open only toasts — a route change there could throw away an unsaved form (a
 * systemd service restarts the daemon while the tab stays open). Absent means "no information",
 * never "cleared". The same crash is reacted to once, across reloads.
 */
export function crashReaction(
  pendingCrashAt: string | null | undefined,
  lastSeen: string | null,
  freshLoad: boolean,
): 'open' | 'toast' | null {
  if (!pendingCrashAt || pendingCrashAt === lastSeen) return null
  return freshLoad ? 'open' : 'toast'
}

export function lastSeenCrash(): string | null {
  try {
    return localStorage.getItem(LAST_CRASH_KEY)
  } catch {
    return null
  }
}

export function rememberSeenCrash(pendingCrashAt: string): void {
  try {
    localStorage.setItem(LAST_CRASH_KEY, pendingCrashAt)
  } catch {
    // Private mode: the crash may be offered again on the next load, which is harmless.
  }
}
