<script lang="ts">
  import { locale } from '../lib/stores/locale.js'
  import * as m from '../lib/i18n/messages.js'

  export let status: string = "stopped";

  const statusClasses = {
    running: "bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200",
    checking: "bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200",
    stopped: "bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200",
  } as const;

  $: classes = statusClasses[status as keyof typeof statusClasses] || statusClasses.stopped;

  $: text = $locale && (
    status === "running" ? m.badge_running() :
    status === "checking" ? m.badge_checking() :
    m.badge_stopped()
  );
</script>

<span class="inline-flex items-center px-2.5 py-0.5 rounded-full font-medium {classes}">
  {text}
</span>
