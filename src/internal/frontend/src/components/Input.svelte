<script lang="ts">
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

    $: inputId = id || `input-${Math.random().toString(36).substr(2, 9)}`;
</script>

<div class="w-full">
    {#if label}
        <label
            for={inputId}
            class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
        >
            {label}
            {#if required}
                <span class="text-red-500 dark:text-red-400">*</span>
            {/if}
        </label>
    {/if}

    {#if subtitle}
        <label
            for={inputId}
            class="block text-xs font-medium text-gray-500 mb-1"
        >
            {subtitle}
        </label>
    {/if}

    <input
        {id}
        {type}
        {placeholder}
        {required}
        {min}
        {max}
        {disabled}
        bind:value
        class="block w-full rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm focus:border-blue-500 focus:ring-blue-500 sm:text-sm disabled:opacity-50 disabled:cursor-not-allowed transition-colors px-3 py-2 {error
            ? 'border-red-300 dark:border-red-600 focus:border-red-500 focus:ring-red-500'
            : ''}"
    />

    {#if error}
        <p class="mt-1 text-sm text-red-600 dark:text-red-400">{error}</p>
    {/if}
</div>
