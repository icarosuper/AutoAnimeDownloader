<script lang="ts">
  // Painel de arquivos de um torrent — spec 2026-08-28. Dono do fetch, do loading e do erro,
  // para que as duas telas que o usam não dupliquem os três.
  //
  // READ-ONLY de propósito: a rain v2 não tem SetWanted/FilePriority (o único `Priority` do
  // módulo é de PEER), então não existe selecionar nem excluir arquivo de dentro do pack.
  //
  // Um endpoint, um componente, DOIS MODOS de render: as duas telas querem recortes diferentes
  // da mesma lista, não listas diferentes.
  import { onDestroy } from "svelte";
  import { getTorrentFiles, type TorrentFile } from "../lib/api/client.js";
  import { filesByEpisode } from "../lib/utils/torrentFiles.js";
  import { formatBytes, formatPercent, type FormatLocale } from "../lib/domain/format.js";
  import ProgressBar from "./ui/ProgressBar.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  export let hash: string;
  /** `raw`: tudo, na ordem do metadata, com o caminho cru. `episodes`: só o que casa episódio. */
  export let mode: "raw" | "episodes" = "raw";
  /** Tick de poll da tela dona. Enquanto o painel está aberto ele se refaz junto; ao fechar,
      o componente é destruído e o refetch para — a lista NUNCA entra no GET /torrents. */
  export let tick = 0;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  let files: TorrentFile[] = [];
  let loaded = false;
  let destroyed = false;

  onDestroy(() => (destroyed = true));

  async function load(h: string) {
    try {
      const next = await getTorrentFiles(h);
      if (destroyed) return;
      files = next;
    } catch {
      // Degrada em silêncio, como o resto da tela: mantém o que já tinha e segue.
    } finally {
      if (!destroyed) loaded = true;
    }
  }

  // `hash` e `tick` como dependências: troca de linha refaz do zero, tick refaz no lugar.
  $: void tick, load(hash);

  $: rows = mode === "episodes" ? filesByEpisode(files) : files;

  // Só o NOME do arquivo: a pasta é a mesma para todos os arquivos do torrent, então repeti-la
  // em cada linha só empurra para fora do truncate exatamente a parte que distingue as linhas —
  // no pack do Erai-raws, com pasta longa, as 13 linhas ficavam visualmente idênticas. O caminho
  // completo continua no `title`.
  const baseName = (p: string) => p.slice(p.lastIndexOf("/") + 1);

  /** null = desconhecido (torrent parado: o rain libera as peças) — "—", nunca 0%. */
  function progressOf(f: TorrentFile): number | null {
    if (f.bytes_completed === null || f.size <= 0) return null;
    return f.bytes_completed / f.size;
  }
</script>

<div class="border-t border-divider bg-control/30 px-4 py-2.5 lg:pl-[94px]">
  {#if rows.length === 0}
    <p class="font-mono text-[12px] text-subtle">
      {loaded ? ($locale && m.files_waiting_metadata()) : "…"}
    </p>
  {:else}
    <ul class="space-y-1.5">
      {#each rows as f (f.path)}
        {@const pct = progressOf(f)}
        <li class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="min-w-0 flex-1 truncate font-mono text-[12px] text-body" title={f.path}>
            {#if mode === "episodes"}
              <span class="font-bold text-heading">{$locale && m.files_ep_label({ number: f.episode ?? 0 })}</span>
              · {baseName(f.path)}
            {:else}
              {baseName(f.path)}
            {/if}
          </span>
          <span class="w-[104px] shrink-0">
            <ProgressBar value={pct ?? 0} thickness={4} variant={pct === 1 ? "ok" : "accent"} label={f.path} />
          </span>
          <span class="shrink-0 font-mono text-[12px] text-subtle">
            {pct === null ? "—" : formatPercent(pct, fmtLocale)} · {formatBytes(f.size, fmtLocale)}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
</div>
