<script lang="ts">
  // ProgressRing — spec §6 (Fase 2). 46px ring via conic-gradient(<color> <pct>%, track 0),
  // with a 34px inner disc showing the percentage in mono. `var(--x)` refs below are the same
  // semantic tokens tokens.css/tailwind.config.js expose as Tailwind classes elsewhere — a
  // conic-gradient can't be expressed with a Tailwind utility class, so this is the one
  // primitive that reaches for the CSS custom property directly instead of a `bg-*` class.
  // That's still "the semantic name", never a literal hex — spec §4.1's constraint is about
  // not hand-writing colors, not about which syntax reaches the token.
  export let value: number // 0..1
  export let variant: 'accent' | 'ok' | 'warn' | 'danger' | 'neutral' = 'accent'
  export let label: string | undefined = undefined

  const COLOR_VAR: Record<string, string> = {
    accent: 'var(--accent)',
    ok: 'var(--ok)',
    warn: 'var(--warn)',
    danger: 'var(--danger)',
    neutral: 'var(--neutral)',
  }

  $: pct = Math.round(Math.max(0, Math.min(1, value || 0)) * 100)
  $: color = COLOR_VAR[variant] ?? COLOR_VAR.accent
</script>

<div
  class="relative flex shrink-0 items-center justify-center rounded-full"
  style="width:46px; height:46px; background: conic-gradient({color} {pct}%, var(--progress-track) 0)"
  role="progressbar"
  aria-valuenow={pct}
  aria-valuemin={0}
  aria-valuemax={100}
  aria-label={label}
>
  <div class="flex items-center justify-center rounded-full bg-card" style="width:34px; height:34px">
    <span class="font-mono text-[10px] font-semibold text-heading">{pct}%</span>
  </div>
</div>
