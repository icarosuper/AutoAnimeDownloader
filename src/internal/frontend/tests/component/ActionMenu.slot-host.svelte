<script lang="ts">
  import ActionMenu from '../../src/components/ui/ActionMenu.svelte'
  import type { ActionMenuItem } from '../../src/components/ui/ActionMenu.svelte'

  export let items: ActionMenuItem[] = []
  export let triggerLabel: string
  export let triggerClass: string | undefined = undefined
</script>

<ActionMenu {items} {triggerLabel} {triggerClass} on:select>
  <span slot="trigger">Airing 12</span>
</ActionMenu>
