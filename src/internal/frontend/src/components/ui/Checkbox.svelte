<script lang="ts">
  import { tick } from "svelte"
  // Checkbox — spec §6 (Phase 2). 16px box, 4px radius; checked = accent fill with a "✓" glyph.
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
   * episode table, Phase 4) still needs a distinct accessible name — "Select episode 3" —
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
  /**
   * Controlled mode: pass `checked` one way and handle the click here; the parent owns the
   * state. After the handler runs, the input is put back to whatever `checked`/`indeterminate`
   * the parent now holds, so a handler that leaves the value unchanged (a select-all over a
   * list a poll just replaced, a toggle that refuses) can't leave the box showing a state
   * the parent doesn't have. Leave it unset for `bind:checked`, where the box owns the click.
   * `on:change` is deliberately not forwarded (decisions/135).
   */
  export let onchange: ((e: Event & { currentTarget: HTMLInputElement }) => void) | undefined = undefined

  let input: HTMLInputElement

  async function handleChange(e: Event & { currentTarget: HTMLInputElement }) {
    if (!onchange) {
      checked = e.currentTarget.checked
      return
    }
    onchange(e)
    await tick()
    input.checked = checked
    input.indeterminate = indeterminate
  }
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
      bind:this={input}
      {checked}
      {indeterminate}
      {disabled}
      on:change={handleChange}
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
