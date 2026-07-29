<script lang="ts">
  // ProgressBar — spec §6+§8 (Fase 2). Single-segment progress bar, `transition: width .9s
  // linear`. `stale` is how §6 and §8 connect: the WebSocket doesn't carry torrent progress
  // (only daemon status), so progress only ever moves on the next successful HTTP poll of
  // GET /api/v1/torrents. When a poll fails, the screen knows — it has the try/catch — and
  // this component does NOT try to guess staleness on its own (e.g. from a timestamp prop or
  // an internal timer): it just takes `stale` as a plain boolean and, when true, kills the
  // transition and dims the fill so a value that silently stopped moving doesn't look like a
  // live number that happens to be flat right now.
  export let value: number // 0..1
  export let stale = false
  /** One of the four thicknesses spec §4.2 fixes: 4 (inline), 6 (card), 7 (group). */
  export let thickness: 4 | 6 | 7 = 6
  export let variant: 'accent' | 'ok' | 'warn' | 'danger' | 'neutral' = 'accent'
  /** Accessible label — no default text: the caller supplies translated copy. */
  export let label: string | undefined = undefined

  const VARIANT_CLASSES: Record<string, string> = {
    accent: 'bg-accent',
    ok: 'bg-ok',
    warn: 'bg-warn',
    danger: 'bg-danger',
    neutral: 'bg-neutral',
  }

  $: pct = Math.max(0, Math.min(1, value || 0)) * 100
  $: barClass = VARIANT_CLASSES[variant] ?? VARIANT_CLASSES.accent
</script>

<div
  class="w-full overflow-hidden rounded-pill bg-track"
  style="height:{thickness}px"
  role="progressbar"
  aria-valuenow={Math.round(pct)}
  aria-valuemin={0}
  aria-valuemax={100}
  aria-label={label}
>
  <div
    class="h-full rounded-pill {barClass} {stale ? 'opacity-50' : ''}"
    style="width:{pct}%; transition:{stale ? 'none' : 'width .9s linear'}"
  ></div>
</div>
