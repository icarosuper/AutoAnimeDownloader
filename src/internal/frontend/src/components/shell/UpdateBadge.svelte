<script lang="ts">
  // Next to the version (NavRail on desktop, MoreMenu on mobile) when the daemon's update check
  // found a newer release. A link to the release page, nothing else: no modal, no dismiss — it goes
  // away on its own once the update is installed, because /status stops reporting it.
  import * as m from '../../lib/i18n/messages.js'
  import { locale } from '../../lib/stores/locale.js'

  export let version = ''
  export let url = ''

  $: label = ($locale && version && m.update_badge_available({ version })) || ''
</script>

{#if version && url}
  <a
    href={url}
    target="_blank"
    rel="noopener noreferrer"
    title={label}
    class="max-w-[82px] rounded-full border border-accent-tint/28 bg-accent-tint/12 px-1.5 py-0.5 text-center text-[10.5px] font-semibold leading-tight text-accent hover:bg-accent-tint/20 transition-colors"
  >
    {label}
  </a>
{/if}
