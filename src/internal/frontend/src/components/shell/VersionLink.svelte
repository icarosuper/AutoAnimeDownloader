<script lang="ts">
  // The version label in the NavRail footer (desktop) and the MoreMenu footer (mobile), doubling as
  // the app's only link to the GitHub repository. The arrow is what tells it apart from plain text.
  import { ArrowUpRight } from '@lucide/svelte'
  import * as m from '../../lib/i18n/messages.js'
  import { REPO_URL } from '../../lib/report.js'
  import { locale } from '../../lib/stores/locale.js'

  export let version = ''

  $: title = ($locale && m.version_link_title()) || ''
</script>

<a
  href={REPO_URL}
  target="_blank"
  rel="noopener noreferrer"
  {title}
  class="inline-flex items-center gap-0.5 text-[11px] text-subtle hover:text-body transition-colors"
>
  v{version}
  <ArrowUpRight size={11} strokeWidth={2} aria-hidden="true" />
</a>
