<script lang="ts">
  import Router from "svelte-spa-router";
  import Layout from "./components/Layout.svelte";
  import Status from "./routes/Status.svelte";
  import AnimeDetail from "./routes/AnimeDetail.svelte";
  import Config from "./routes/Config.svelte";
  import Priorities from "./routes/Priorities.svelte";
  import Logs from "./routes/Logs.svelte";
  import Notifications from "./routes/Notifications.svelte";
  import Downloads from "./routes/Downloads.svelte";

  const routes: Record<string, unknown> = {
    "/": Status,
    "/status": Status,
    "/status/:id": AnimeDetail,
    "/downloads": Downloads,
    "/config": Config,
    "/priorities": Priorities,
    "/logs": Logs,
    "/notifications": Notifications,
  };
</script>

<Layout>
  <Router {routes} />
</Layout>
