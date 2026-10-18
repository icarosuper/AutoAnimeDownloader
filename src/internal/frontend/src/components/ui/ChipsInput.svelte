<script lang="ts">
  // ChipsInput — spec §9.4 (Phase 6). The multi-value input from artboard 1e: chips live
  // INSIDE the field, with the cursor in the same box, and there is no "+" button — the
  // value comes in from the keyboard (Enter/comma) or by pasting a list.
  //
  // The five keyboard rules from the spec, all covered by tests/component/ChipsInput.test.ts:
  //   1. Enter commits           2. comma commits
  //   3. Backspace on an empty field removes the last
  //   4. clicking × removes      5. pasting a comma-separated list creates several
  //
  // The `<input>` is a sibling of the chips inside a flex-wrap, not a separate field below
  // them: that's what makes the cursor land "in the same box" and the field grow as it fills.
  // The whole box gets the focus ring via `focus-within`, since the actually focused element
  // is the inner input and not the border the user sees.
  import { X } from '@lucide/svelte'

  export let values: string[] = []
  /** Visible label, associated with the real input via `for`/`id` — never a loose <label>. */
  export let label: string
  export let hint = ''
  export let placeholder = ''
  export let id: string = `chips-${Math.random().toString(36).slice(2, 9)}`
  export let disabled = false
  /**
   * Marks the field as required: asterisk on the label + `aria-required` on the input. It is
   * not the native `required` attribute because what is required here is the LIST, not the
   * text being typed — with a chip already committed the input is empty, and native `required`
   * would fail a field that is filled.
   */
  export let required = false
  /** Accessible name of each chip's ×; receives the chip value as `{item}`. */
  export let removeLabel: (item: string) => string = (item) => `Remove ${item}`

  let draft = ''
  let inputEl: HTMLInputElement

  /**
   * A raw entry can become SEVERAL values (the paste case). Splitting on comma here, not
   * only in the paste handler, is what makes "a,b,c" + Enter behave the same as pasting
   * "a,b,c" — one function, one path.
   */
  function commit(raw: string) {
    const parts = raw
      .split(',')
      .map((p) => p.trim())
      .filter((p) => p.length > 0)
    if (parts.length === 0) return
    // Deduplicate against what already exists AND within the pasted batch itself.
    const next = [...values]
    for (const part of parts) {
      if (!next.includes(part)) next.push(part)
    }
    values = next
  }

  function commitDraft() {
    commit(draft)
    draft = ''
  }

  function remove(item: string) {
    values = values.filter((v) => v !== item)
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Enter' || event.key === ',') {
      // Enter inside a <form> would submit the whole screen (Config has a real Save button);
      // the comma would be typed literally. Both need to be intercepted.
      event.preventDefault()
      commitDraft()
      return
    }
    if (event.key === 'Backspace' && draft === '' && values.length > 0) {
      // Only remove with the field EMPTY — with typed text, Backspace keeps deleting letters.
      event.preventDefault()
      values = values.slice(0, -1)
    }
  }

  function handlePaste(event: ClipboardEvent) {
    const text = event.clipboardData?.getData('text') ?? ''
    if (!text.includes(',')) return // pasting a single value follows the normal typing flow
    event.preventDefault()
    commit(text)
  }
</script>

<div class="flex flex-col gap-1.5">
  <label for={id} class="text-[14.5px] font-bold text-heading">
    {label}
    {#if required}
      <!-- Same pair Input.svelte uses: visual asterisk (aria-hidden, the screen reader
           gets the information via `aria-required`) in the danger color. -->
      <span class="text-danger" aria-hidden="true">*</span>
    {/if}
  </label>

  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <!-- Clicking the empty area of the box focuses the input, like a normal text field. The
       keyboard doesn't need this shortcut: the real input is already in the tab order. -->
  <div
    class="flex flex-wrap items-center gap-1.5 rounded-field border border-default bg-control px-2.5 py-2 focus-within:border-accent {disabled
      ? 'cursor-not-allowed opacity-50'
      : ''}"
    on:click={() => !disabled && inputEl?.focus()}
  >
    {#each values as item (item)}
      <span
        class="inline-flex items-center gap-1 rounded-pill border border-accent-tint/28 bg-accent-tint/12 px-2 py-0.5 text-caption font-semibold text-accent"
      >
        {item}
        <button
          type="button"
          {disabled}
          class="text-accent/70 transition-colors hover:text-accent disabled:cursor-not-allowed"
          aria-label={removeLabel(item)}
          on:click|stopPropagation={() => remove(item)}
        >
          <X size={12} strokeWidth={2.5} />
        </button>
      </span>
    {/each}

    <input
      {id}
      bind:this={inputEl}
      bind:value={draft}
      {placeholder}
      {disabled}
      aria-required={required ? 'true' : undefined}
      type="text"
      class="min-w-[8rem] flex-1 bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle disabled:cursor-not-allowed"
      on:keydown={handleKeydown}
      on:paste={handlePaste}
      on:blur={commitDraft}
    />
  </div>

  {#if hint}
    <p class="text-caption text-subtle">{hint}</p>
  {/if}
</div>
