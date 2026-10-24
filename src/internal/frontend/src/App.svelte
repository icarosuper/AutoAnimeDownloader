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
  import AddAnime from "./routes/AddAnime.svelte";
  import TorrentPicker from "./routes/TorrentPicker.svelte";
  import Report from "./routes/Report.svelte";

  // Longer paths first: svelte-spa-router walks insertion order, so `/status/:id` would
  // swallow `/status/:id/torrents` if it were registered first.
  const routes: Record<string, unknown> = {
    "/": Status,
    "/status/:id/torrents": TorrentPicker,
    "/status/:id": AnimeDetail,
    "/status": Status,
    "/add/:id/torrents": TorrentPicker,
    "/add": AddAnime,
    "/downloads": Downloads,
    "/config": Config,
    "/priorities": Priorities,
    "/logs": Logs,
    "/notifications": Notifications,
    "/report": Report,
  };
</script>

<AppShell>
  <Router {routes} />
</AppShell>
