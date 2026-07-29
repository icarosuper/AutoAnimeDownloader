<script lang="ts">
    // Input — mantido e reestilizado na Fase 6 (spec §6: os componentes existentes são
    // reestilizados, não substituídos). A API pública não mudou; só as classes saíram dos
    // hexes literais do Tailwind (`bg-white dark:bg-gray-700`, `focus:ring-blue-500`) para os
    // tokens semânticos do §4.1.
    //
    // A anatomia agora é a do campo do §9.4: rótulo 13.5px/700, controle, e uma linha de ajuda
    // 12px em --text-subtle. O `subtitle` era renderizado como um SEGUNDO <label> apontando
    // para o mesmo input — dois rótulos para um controle. Virou <p>, que é o que ele sempre
    // foi semanticamente, e o input passa a referenciá-lo por aria-describedby.
    export let id: string = "";
    export let label: string = "";
    export let subtitle: string = "";
    export let type: string = "text";
    export let value: string | number = "";
    export let placeholder: string = "";
    export let required: boolean = false;
    export let min: number | string | null = null;
    export let max: number | string | null = null;
    export let disabled: boolean = false;
    export let error: string = "";

    $: inputId = id || `input-${Math.random().toString(36).slice(2, 11)}`;
    $: hintId = `${inputId}-hint`;
    $: errorId = `${inputId}-error`;
    // Só descreve o que existe: um id apontando para um elemento ausente é uma referência
    // quebrada na árvore de acessibilidade.
    $: describedBy = [subtitle ? hintId : null, error ? errorId : null].filter(Boolean).join(" ") || undefined;
</script>

<div class="flex w-full flex-col gap-1.5">
    {#if label}
        <label for={inputId} class="text-[14.5px] font-bold text-heading">
            {label}
            {#if required}
                <span class="text-danger" aria-hidden="true">*</span>
            {/if}
        </label>
    {/if}

    <!-- `type` não pode ser passado como atributo dinâmico junto de bind:value no Svelte, daí
         os dois ramos abaixo em vez de um <input {type} bind:value>. -->
    {#if type === "number"}
        <input
            id={inputId}
            type="number"
            {placeholder}
            {required}
            {min}
            {max}
            {disabled}
            aria-describedby={describedBy}
            aria-invalid={error ? "true" : undefined}
            bind:value
            class="w-full rounded-field border bg-control px-3 py-2 font-mono text-copy text-heading outline-none transition-colors placeholder:font-sans placeholder:font-normal placeholder:text-subtle focus:border-accent disabled:cursor-not-allowed disabled:opacity-50 {error
                ? 'border-danger'
                : 'border-default'}"
        />
    {:else}
        <input
            id={inputId}
            {type}
            {placeholder}
            {required}
            {min}
            {max}
            {disabled}
            aria-describedby={describedBy}
            aria-invalid={error ? "true" : undefined}
            bind:value
            class="w-full rounded-field border bg-control px-3 py-2 text-copy text-heading outline-none transition-colors placeholder:font-normal placeholder:text-subtle focus:border-accent disabled:cursor-not-allowed disabled:opacity-50 {error
                ? 'border-danger'
                : 'border-default'}"
        />
    {/if}

    {#if subtitle}
        <p id={hintId} class="text-caption text-subtle">{subtitle}</p>
    {/if}

    {#if error}
        <p id={errorId} class="text-caption text-danger">{error}</p>
    {/if}
</div>
