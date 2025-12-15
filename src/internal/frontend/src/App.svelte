<script lang="ts">
  import Router from "svelte-spa-router";
  import Layout from "./components/Layout.svelte";
  import Status from "./routes/Status.svelte";
  import Episodes from "./routes/Episodes.svelte";
  import Config from "./routes/Config.svelte";

  const routes: Record<string, unknown> = {
    "/": Status,
    "/status": Status,
    "/episodes": Episodes,
    "/config": Config,
  };
</script>

<Layout>
  <Router {routes} />
</Layout>
