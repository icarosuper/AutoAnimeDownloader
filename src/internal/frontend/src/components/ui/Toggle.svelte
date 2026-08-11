<script lang="ts">
  // Toggle — spec §6 (Fase 2), paired with Checkbox. Same accessibility approach: a real
  // `<input type="checkbox" role="switch">` under a styled track/thumb, so Space/Tab and label
  // association come from the browser instead of hand-rolled key handling.
  export let checked = false
  export let disabled = false
  /** Required, no default — spec calls for an accessible label on every instance. */
  export let label: string
  export let id: string = `toggle-${Math.random().toString(36).slice(2, 9)}`
  /**
   * Linha de duas colunas: rótulo à esquerda, chave à direita. `flex-row-reverse` inverte a
   * ORDEM VISUAL sem tocar na ordem do DOM, então a associação rótulo↔input e a ordem de
   * tabulação continuam as mesmas.
   */
  export let inline = false
</script>

<label
  for={id}
  class="select-none items-center gap-2 {inline
    ? 'flex w-full flex-row-reverse justify-between'
    : 'inline-flex'} {disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}"
>
  <span
    class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-pill transition-colors {checked
      ? 'bg-accent'
      : 'bg-control'}"
  >
    <input
      {id}
      type="checkbox"
      role="switch"
      bind:checked
      {disabled}
      on:change
      class="absolute inset-0 h-full w-full cursor-pointer opacity-0 disabled:cursor-not-allowed"
    />
    <span
      class="pointer-events-none inline-block h-4 w-4 translate-x-0.5 transform rounded-full bg-card transition-transform {checked
        ? 'translate-x-[18px]'
        : ''}"
      aria-hidden="true"
    ></span>
  </span>
  <span class="text-copy text-body">{label}</span>
</label>
