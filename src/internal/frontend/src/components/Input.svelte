<script lang="ts">
    // Input — kept and restyled in Phase 6 (spec §6: existing components are restyled, not
    // replaced). The public API didn't change; only the classes left Tailwind's literal hexes
    // (`bg-white dark:bg-gray-700`, `focus:ring-blue-500`) for the semantic tokens of §4.1.
    //
    // The anatomy is now the §9.4 field: 13.5px/700 label, control, and a 12px help line in
    // --text-subtle. `subtitle` used to render as a SECOND <label> pointing at the same
    // input — two labels for one control. It became a <p>, which is what it always was
    // semantically, and the input now references it via aria-describedby.
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
     * Two-column row: label and hint on the left, control on the right with width
     * proportional to the data. Opt-in — `Notifications.svelte` also consumes this component
     * and stays on the stacked row.
     *
     * The layout comes only from explicit grid placement over the markup that already exists.
     * Branching the markup would take the two <input> branches (already two because of
     * Svelte's limitation with dynamic `type` + bind:value) to four.
     */
    export let inline: boolean = false;
    /** Unit next to the control (e.g. "min", "%", "GiB"). aria-hidden: it's already in the hint. */
    export let suffix: string = "";

    $: inputClass = `${inline ? "w-full md:w-24" : "w-full"} rounded-field border bg-control px-3 py-2 text-copy text-heading outline-none transition-colors placeholder:font-normal placeholder:text-subtle focus:border-accent disabled:cursor-not-allowed disabled:opacity-50 ${error ? "border-danger" : "border-default"}`;

    $: inputId = id || `input-${Math.random().toString(36).slice(2, 11)}`;
    $: hintId = `${inputId}-hint`;
    $: errorId = `${inputId}-error`;
    // Only describe what exists: an id pointing at a missing element is a broken
    // reference in the accessibility tree.
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

    <!-- Permanent wrapper (not only in inline): it hosts the suffix and the grid placement.
         Without `suffix` and without `inline` it is transparent — the input still takes 100%. -->
    <div class="flex w-full items-center gap-2 {inline ? 'md:col-start-2 md:row-start-1 md:w-auto' : ''}">
        <!-- `type` cannot be passed as a dynamic attribute together with bind:value in Svelte,
             hence the two branches below instead of one <input {type} bind:value>. -->
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
