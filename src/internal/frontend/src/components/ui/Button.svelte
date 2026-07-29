<script lang="ts">
  // Button — spec §6 (Fase 2). Three variants: `solid` (the accent, "moves the screen
  // forward" action), `ghost` (neutral secondary action), `warn` (destructive/attention,
  // tinted danger — NOT a solid danger fill). `episodeActions.ts` (lib/domain/) returns one of
  // these three strings per action, so a screen can drive this component straight from that
  // data without a translation step.
  //
  // Spec §4.1: at most one solid accent per screen. This component doesn't enforce that (it
  // can't know what else is on the screen) — it exists so screens only have ONE variant to
  // reach for when they need "the" primary action, instead of each screen inventing its own
  // solid-button styling and accidentally using it twice.
  export let variant: 'solid' | 'ghost' | 'warn' = 'ghost'
  export let type: 'button' | 'submit' = 'button'
  export let disabled = false
  /** Accessible name when the button is icon-only (no visible text in the slot). */
  export let ariaLabel: string | undefined = undefined

  const VARIANT_CLASSES: Record<string, string> = {
    solid: 'bg-accent text-on-accent hover:opacity-90',
    ghost: 'border border-default bg-transparent text-body hover:bg-control',
    warn: 'border border-danger-tint/32 bg-danger-tint/12 text-danger hover:bg-danger-tint/16',
  }

  $: classes = VARIANT_CLASSES[variant] ?? VARIANT_CLASSES.ghost
</script>

<button
  {type}
  {disabled}
  aria-label={ariaLabel}
  class="inline-flex items-center justify-center gap-1.5 rounded-control px-3 py-1.5 text-copy font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50 {classes}"
  on:click
>
  <slot />
</button>
