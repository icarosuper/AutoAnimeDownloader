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
    export let step: number | string | null = null;
    export let disabled: boolean = false;
    export let error: string = "";
    /**
     * Linha de duas colunas: rótulo e dica à esquerda, controle à direita com largura
     * proporcional ao dado. Opt-in — `Notifications.svelte` também consome este componente e
     * continua na linha empilhada.
     *
     * O layout sai só de colocação explícita de grid sobre a marcação que já existe. Ramificar a
     * marcação levaria os dois ramos de <input> (que já são dois por causa da limitação do Svelte
     * com `type` dinâmico + bind:value) para quatro.
     */
    export let inline: boolean = false;
    /** Unidade ao lado do controle (ex. "min", "%", "GiB"). aria-hidden: já está na dica. */
    export let suffix: string = "";

    $: inputClass = `${inline ? "w-full md:w-24" : "w-full"} rounded-field border bg-control px-3 py-2 text-copy text-heading outline-none transition-colors placeholder:font-normal placeholder:text-subtle focus:border-accent disabled:cursor-not-allowed disabled:opacity-50 ${error ? "border-danger" : "border-default"}`;

    $: inputId = id || `input-${Math.random().toString(36).slice(2, 11)}`;
    $: hintId = `${inputId}-hint`;
    $: errorId = `${inputId}-error`;
    // Só descreve o que existe: um id apontando para um elemento ausente é uma referência
    // quebrada na árvore de acessibilidade.
    $: describedBy = [subtitle ? hintId : null, error ? errorId : null].filter(Boolean).join(" ") || undefined;
</script>

<div
    class="flex w-full flex-col gap-1.5 {inline
        ? 'md:grid md:grid-cols-[1fr_auto] md:items-start md:gap-x-4 md:gap-y-1'
        : ''}"
>
    {#if label}
        <label for={inputId} class="text-[14.5px] font-bold text-heading {inline ? 'md:col-start-1 md:row-start-1' : ''}">
            {label}
            {#if required}
                <span class="text-danger" aria-hidden="true">*</span>
            {/if}
        </label>
    {/if}

    <!-- Wrapper permanente (não só no inline): é o que hospeda o sufixo e a colocação de grid.
         Sem `suffix` e sem `inline` ele é transparente — o input continua ocupando 100%. -->
    <div class="flex w-full items-center gap-2 {inline ? 'md:col-start-2 md:row-start-1 md:w-auto' : ''}">
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
                {step}
                {disabled}
                aria-describedby={describedBy}
                aria-invalid={error ? "true" : undefined}
                bind:value
                class="{inputClass} font-mono placeholder:font-sans"
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
                class={inputClass}
            />
        {/if}

        {#if suffix}
            <span class="shrink-0 text-caption text-subtle" aria-hidden="true">{suffix}</span>
        {/if}
    </div>

    {#if subtitle}
        <p id={hintId} class="text-caption text-subtle {inline ? 'md:col-start-1 md:row-start-2' : ''}">{subtitle}</p>
    {/if}

    {#if error}
        <p id={errorId} class="text-caption text-danger {inline ? 'md:col-start-1 md:row-start-3' : ''}">{error}</p>
    {/if}
</div>
