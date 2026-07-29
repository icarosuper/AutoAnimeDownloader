<script lang="ts">
  import Router from "svelte-spa-router";
  import AppShell from "./components/shell/AppShell.svelte";
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

<AppShell>
  <Router {routes} />
</AppShell>
