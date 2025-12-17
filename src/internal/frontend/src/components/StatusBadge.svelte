<script lang="ts">
  export let status: string = "stopped";

  const statusConfigs = {
    running: {
      text: "Running",
      classes:
        "bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200",
    },
    checking: {
      text: "Checking",
      classes:
        "bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200",
    },
    stopped: {
      text: "Stopped",
      classes: "bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200",
    },
  } as const;

  $: badgeConfig =
    statusConfigs[status as keyof typeof statusConfigs] ||
    statusConfigs.stopped;
</script>

<span
  class="inline-flex items-center px-2.5 py-0.5 rounded-full font-medium {badgeConfig.classes}"
>
  {badgeConfig.text}
</span>
