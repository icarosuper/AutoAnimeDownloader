<script lang="ts">
  // Checkbox — spec §6 (Fase 2). 16px box, 4px radius; checked = accent fill with a "✓" glyph.
  // Built on a real `<input type="checkbox">` (visually hidden, stacked under the custom box)
  // rather than a `<div role="checkbox">` + manual key handling — that's what gives it native
  // keyboard operability (Space toggles, Tab focuses) and a programmatically associated label
  // for free, satisfying spec's accessibility requirement without reinventing it.
  export let checked = false
  export let disabled = false
  /** Required, no default — spec calls for an accessible label on every instance. */
  export let label: string
  export let id: string = `checkbox-${Math.random().toString(36).slice(2, 9)}`
  /**
   * Renders `label` for screen readers only. A per-row selection checkbox (AnimeDetail's
   * episode table, Fase 4) still needs a distinct accessible name — "Selecionar episódio 3" —
   * but printing that next to all 12 boxes would be noise. `label` stays REQUIRED either way,
   * so hiding it can never become a way to ship an unlabelled checkbox.
   */
  export let labelHidden = false
  /**
   * The "some but not all selected" dash on a select-all box. `indeterminate` is a DOM
   * property with no HTML attribute equivalent, so it only works bound on the real <input>
   * below — which is exactly why this primitive wraps a native checkbox instead of a div.
   */
  export let indeterminate = false
</script>

<label
  for={id}
  class="inline-flex select-none items-center gap-2 {disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}"
>
  <span
    class="relative inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-[4px] border {checked ||
    indeterminate
      ? 'border-accent bg-accent'
      : 'border-default bg-control'}"
  >
    <input
      {id}
      type="checkbox"
      bind:checked
      {indeterminate}
      {disabled}
      on:change
      class="absolute inset-0 h-full w-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
    />
    {#if checked}
      <span class="pointer-events-none font-mono text-[11px] leading-none text-on-accent" aria-hidden="true">✓</span>
    {:else if indeterminate}
      <span class="pointer-events-none h-[2px] w-2 rounded-pill bg-on-accent" aria-hidden="true"></span>
    {/if}
  </span>
  <span class={labelHidden ? 'sr-only' : 'text-copy text-body'}>{label}</span>
</label>
