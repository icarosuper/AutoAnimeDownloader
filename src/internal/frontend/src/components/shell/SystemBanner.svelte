<script lang="ts">
  // System banner — ONE only, with precedence, hosted in the AppShell (spec: decisions.md #66).
  // It's not a toast: a toast is the failure of an ACTION the user asked for and goes away on its
  // own; the banner is a DEGRADED state that persists across polls and explains why the whole
  // screen looks wrong. Its value isn't reporting the error — it's saying why the list is stale or
  // incomplete, which the user would otherwise conclude on their own (wrongly) as "animes vanished".
  //
  // Its /status poll also watches `pending_crash_at`: the shell's only status poller, and the first
  // answer after mount is what tells a fresh page load from a tab that was already open.
  import { onDestroy, onMount } from 'svelte'
  import { push } from 'svelte-spa-router'
  import { AlertTriangle } from '@lucide/svelte'
  import { getStatus, type AnilistHealth } from '../../lib/api/client.js'
  import { backendHealth } from '../../lib/stores/backendHealth.js'
  import { pickBanner, secondsUntil } from '../../lib/domain/systemBanner.js'
  import { crashReaction, lastSeenCrash, rememberSeenCrash, reportHref } from '../../lib/report.js'
  import { toast } from '../../lib/stores/toast.js'
  import { locale } from '../../lib/stores/locale.js'
  import * as m from '../../lib/i18n/messages.js'

  // Polled only to feed the banner. Slow on purpose: the state it shows changes on a scale of
  // minutes (outage, the rate limit's 1-minute timeout), not seconds.
  const STATUS_POLL_MS = 30000
  // The crash toast stays until it's read: the daemon restarted behind the user's back.
  const CRASH_TOAST_MS = 60 * 60 * 1000

  let anilist: AnilistHealth | null = null
  let now = Date.now()
  let pollId: ReturnType<typeof setInterval> | null = null
  let tickId: ReturnType<typeof setInterval> | null = null
  let freshLoad = true

  async function loadStatus() {
    try {
      // silent: a failure here is already counted by backendHealth and becomes the banner itself.
      // A toast every 30s with the daemon down would be just noise.
      const status = await getStatus({ silent: true })
      anilist = status.anilist ?? null
      reactToCrash(status.pending_crash_at)
    } catch {
      // The AniList state keeps its last known value; the banner's precedence guarantees it isn't
      // shown while the backend is down — it would be stale information.
    }
  }

  function reactToCrash(pendingCrashAt: string | null | undefined) {
    const reaction = crashReaction(pendingCrashAt, lastSeenCrash(), freshLoad)
    freshLoad = false
    if (!reaction || !pendingCrashAt) return
    rememberSeenCrash(pendingCrashAt)
    if (reaction === 'open') {
      push('/report?crash=1')
    } else {
      toast.error(m.report_crash_toast(), CRASH_TOAST_MS, { href: '#/report?crash=1', label: m.report_toast_link() })
    }
  }

  onMount(() => {
    loadStatus()
    pollId = setInterval(loadStatus, STATUS_POLL_MS)
    // Second timer only for the rate limit countdown, the one case where the exact remaining time
    // is known (Retry-After).
    tickId = setInterval(() => (now = Date.now()), 1000)
  })

  onDestroy(() => {
    if (pollId) clearInterval(pollId)
    if (tickId) clearInterval(tickId)
  })

  $: banner = pickBanner($backendHealth, anilist)
  $: seconds = banner ? secondsUntil(banner.retryAt, now) : 0
  // Short date without seconds: what the user needs to know is whether the cache is from today or
  // last week, not the exact moment it was written.
  $: cacheText =
    $locale && banner?.cachedAt
      ? m.banner_serving_cache({
          date: banner.cachedAt.toLocaleString($locale, { dateStyle: 'short', timeStyle: 'short' }),
        })
      : ''

  $: text =
    $locale && banner
      ? banner.kind === 'backend_unreachable'
        ? m.banner_backend_unreachable()
        : banner.kind === 'backend_error'
          ? m.banner_backend_error()
          : banner.kind === 'anilist_outage'
            ? m.banner_anilist_outage()
            : banner.kind === 'anilist_app_bug'
              ? m.banner_anilist_app_bug()
              : seconds > 0
                ? m.banner_anilist_rate_limited({ seconds })
                : m.banner_anilist_rate_limited_no_eta()
      : ''

  // The report screen, pre-filled with the banner: it adds the redacted log and environment that a
  // bare GitHub link never carried.
  $: bannerReportHref = banner
    ? reportHref({ error: banner.detail ?? text, context: `banner: ${banner.kind}` })
    : ''
</script>

{#if banner}
  <!-- role="alert" and not "status": the content below the banner is wrong or frozen, and the
       screen reader needs to know that before reading on. -->
  <div
    role="alert"
    data-testid="system-banner"
    data-banner-kind={banner.kind}
    class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-4 py-2.5 text-copy sm:px-6 lg:px-8 {banner.reportable
      ? 'border-danger-tint/32 bg-danger-tint/12 text-danger'
      : 'border-warn-tint/32 bg-warn-tint/12 text-warn'}"
  >
    <AlertTriangle class="h-4 w-4 shrink-0" aria-hidden="true" />
    <span class="min-w-0 flex-1">
      {text}
      {#if cacheText}
        <span class="block text-caption opacity-80">{cacheText}</span>
      {/if}
      {#if banner.detail}
        <!-- AniList's raw message. A blocked-IP 403 explains the reason in writing and is the only
             information the frontend can't rebuild — shown verbatim. -->
        <span class="block text-caption opacity-80">{banner.detail}</span>
      {/if}
    </span>
    {#if banner.reportable}
      <a
        href={bannerReportHref}
        class="inline-flex shrink-0 items-center gap-1 underline underline-offset-2 hover:opacity-80"
      >
        {$locale && m.banner_report()}
      </a>
    {/if}
  </div>
{/if}
