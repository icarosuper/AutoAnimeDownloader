<script lang="ts">
  // ChipsInput — spec §9.4 (Fase 6). O input multi-valor do artboard 1e: os chips vivem
  // DENTRO do campo, com o cursor na mesma caixa, e não existe botão "+" — o valor entra
  // por teclado (Enter/vírgula) ou colando uma lista.
  //
  // As cinco regras de teclado do spec, todas cobertas por tests/component/ChipsInput.test.ts:
  //   1. Enter confirma          2. vírgula confirma
  //   3. Backspace com campo vazio remove o último
  //   4. clicar no × remove      5. colar lista separada por vírgulas cria vários
  //
  // O `<input>` é um irmão dos chips dentro de um flex-wrap, não um campo separado embaixo
  // deles: é isso que faz o cursor cair "na mesma caixa" e o campo crescer conforme enche.
  // A caixa inteira ganha o anel de foco via `focus-within`, já que o elemento realmente
  // focado é o input interno e não a borda que o usuário enxerga.
  import { X } from '@lucide/svelte'

  export let values: string[] = []
  /** Rótulo visível, associado ao input real via `for`/`id` — nunca um <label> solto. */
  export let label: string
  export let hint = ''
  export let placeholder = ''
  export let id: string = `chips-${Math.random().toString(36).slice(2, 9)}`
  export let disabled = false
  /** Nome acessível do × de cada chip; recebe o valor do chip como `{item}`. */
  export let removeLabel: (item: string) => string = (item) => `Remove ${item}`

  let draft = ''
  let inputEl: HTMLInputElement

  /**
   * Uma entrada crua pode virar VÁRIOS valores (o caso do paste). Separar por vírgula aqui,
   * e não só no handler de paste, é o que faz "a,b,c" + Enter se comportar igual a colar
   * "a,b,c" — mesma função, um caminho só.
   */
  function commit(raw: string) {
    const parts = raw
      .split(',')
      .map((p) => p.trim())
      .filter((p) => p.length > 0)
    if (parts.length === 0) return
    // Deduplica contra o que já existe E dentro do próprio lote colado.
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
      // Enter dentro de um <form> submeteria a tela inteira (Config tem um botão Salvar de
      // verdade); a vírgula seria digitada literalmente. Os dois precisam ser interceptados.
      event.preventDefault()
      commitDraft()
      return
    }
    if (event.key === 'Backspace' && draft === '' && values.length > 0) {
      // Só remove com o campo VAZIO — com texto digitado, Backspace continua apagando letra.
      event.preventDefault()
      values = values.slice(0, -1)
    }
  }

  function handlePaste(event: ClipboardEvent) {
    const text = event.clipboardData?.getData('text') ?? ''
    if (!text.includes(',')) return // colar um valor só segue o fluxo normal de digitação
    event.preventDefault()
    commit(text)
  }
</script>

<div class="flex flex-col gap-1.5">
  <label for={id} class="text-[13.5px] font-bold text-heading">{label}</label>

  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <!-- Clicar na área vazia da caixa foca o input, como num campo de texto comum. O teclado
       não precisa deste atalho: o input real já está na ordem de tabulação. -->
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
