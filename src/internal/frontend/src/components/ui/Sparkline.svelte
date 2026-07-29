<script lang="ts">
  // Sparkline — spec §6+§8 (Fase 2). 20 bars, 7px wide, 3px gap, 64px max height. Reads
  // straight off `lib/stores/speedHistory.ts` (`$speedHistory`, up to 20 samples, oldest
  // first) — pass that array in as `values`; when a poll fails the store simply isn't pushed
  // to, so the same array (and therefore the same bars) is what re-renders. Nothing here
  // detects staleness itself, same principle as ProgressBar.
  export let values: number[] = []
  /** Fixed scale ceiling. Omit to auto-scale to the tallest sample currently in `values`. */
  export let max: number | undefined = undefined
  export let variant: 'accent' | 'ok' | 'warn' | 'danger' | 'neutral' = 'accent'
  export let label: string | undefined = undefined

  const BAR_COUNT = 20
  const MAX_HEIGHT = 64
  const MIN_HEIGHT = 2 // a zero sample still renders a sliver, not a fully collapsed bar

  const VARIANT_CLASSES: Record<string, string> = {
    accent: 'bg-accent',
    ok: 'bg-ok',
    warn: 'bg-warn',
    danger: 'bg-danger',
    neutral: 'bg-neutral',
  }

  $: peak = max ?? Math.max(1, ...values)
  // Right-aligned: the most recent sample is always the last (rightmost) bar. Slots with no
  // sample yet (fewer than 20 collected so far) render as empty track, not zero-height bars.
  $: bars = Array.from({ length: BAR_COUNT }, (_, i) => {
    const idx = values.length - BAR_COUNT + i
    return idx >= 0 ? values[idx] : undefined
  })
  $: barClass = VARIANT_CLASSES[variant] ?? VARIANT_CLASSES.accent
</script>

<div
  class="flex items-end gap-[3px]"
  style="height:{MAX_HEIGHT}px"
  role={label ? 'img' : undefined}
  aria-label={label}
>
  {#each bars as v, i (i)}
    <div
      class="w-[7px] rounded-t-sm {v === undefined ? 'bg-track' : barClass}"
      style="height:{v === undefined ? MIN_HEIGHT : Math.max(MIN_HEIGHT, (v / peak) * MAX_HEIGHT)}px"
    ></div>
  {/each}
</div>
