<script lang="ts">
  import Router from 'svelte-spa-router'
  import Layout from './components/Layout.svelte'
  import Status from './routes/Status.svelte'
  import Episodes from './routes/Episodes.svelte'
  import Config from './routes/Config.svelte'
  import type { ComponentType } from 'svelte'

  const routes: Record<string, ComponentType> = {
    '/': Status,
    '/status': Status,
    '/episodes': Episodes,
    '/config': Config,
  }
</script>

<Layout>
  <Router {routes} />
</Layout>

