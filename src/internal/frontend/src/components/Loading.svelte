<script lang="ts">
  export let message: string = 'Loading...'
</script>

<div class="flex items-center justify-center p-8">
  <div class="text-center">
    <div class="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 dark:border-blue-400"></div>
    {#if message}
      <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">{message}</p>
    {/if}
  </div>
</div>

