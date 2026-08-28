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
  import Chip from "./ui/Chip.svelte";
  import ProgressBar from "./ui/ProgressBar.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  export let hash: string;
  /** `raw`: tudo, na ordem do metadata, com o nome cru. `episodes`: só o que casa episódio. */
  export let mode: "raw" | "episodes" = "raw";
  /** Tick de poll da tela dona. Enquanto o painel está aberto ele se refaz junto; ao fechar,
      o componente é destruído e o refetch para — a lista NUNCA entra no GET /torrents. */
  export let tick = 0;
  /**
   * Classe de grid das linhas da tela dona (o `EP_GRID` do AnimeDetail). Com ela, o arquivo
   * usa EXATAMENTE as mesmas colunas do episódio avulso — #, episódio, estado, detalhes,
   * ações — em vez de um segundo estilo de lista logo abaixo do primeiro. Vazio = layout
   * próprio, compacto (Downloads e o empilhado do mobile).
   */
  export let gridClass = "";
  /**
   * Chip do torrent dono, para a coluna ESTADO das linhas que ainda não terminaram: o estado
   * delas É o do pack (pausado, baixando, na fila). Arquivo completo mostra "Baixado" e não
   * depende disto.
   */
  export let chip: { variant: "accent" | "ok" | "warn" | "danger" | "neutral"; label: string } | undefined =
    undefined;

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

  /** "48% · 1,4 GB", ou "— · 1,4 GB" quando o progresso por arquivo é desconhecido. */
  function detailsOf(f: TorrentFile): string {
    const pct = progressOf(f);
    return `${pct === null ? "—" : formatPercent(pct, fmtLocale)} · ${formatBytes(f.size, fmtLocale)}`;
  }

  function chipFor(f: TorrentFile) {
    if (progressOf(f) === 1) return { variant: "ok" as const, label: m.detail_badge_downloaded() };
    return chip;
  }
</script>

{#if gridClass}
  <!-- Modo tabela: as linhas de arquivo repetem as colunas da tela dona, com o fundo levemente
       tingido como única marca de que são o nível de dentro do accordion. -->
  <div class="bg-control/30">
    {#if rows.length === 0}
      <p class="px-4 py-2.5 font-mono text-[12px] text-subtle">
        {loaded ? ($locale && m.files_waiting_metadata()) : "…"}
      </p>
    {:else}
      {#each rows as f (f.path)}
        {@const pct = progressOf(f)}
        {@const c = chipFor(f)}
        {@const details = detailsOf(f)}
        <div class="{gridClass} border-t border-divider px-4 py-2.5">
          <span></span>
          <!-- "Episódio N", não o nome do arquivo: a tela é sobre episódio, e a linha tem que
               ler igual à do episódio avulso logo acima. O nome cru continua no `title` (e
               inteiro, sempre, na lista das Downloads). O recuo é a única marca de que a linha
               é o nível de dentro do pack. -->
          <p class="truncate pl-5 text-copy text-heading" title={f.path}>
            {$locale && m.detail_ep_title({ number: f.episode ?? 0 })}
          </p>
          <div class="min-w-0">
            {#if c}
              <Chip variant={c.variant}>{c.label}</Chip>
            {/if}
            {#if pct !== null && pct < 1}
              <div class="mt-1.5">
                <ProgressBar value={pct} thickness={4} label={f.path} />
              </div>
            {/if}
          </div>
          <span class="truncate font-mono text-[12px] text-subtle" title={details}>{details}</span>
          <!-- Coluna de ações vazia: o painel é read-only (a rain não seleciona arquivo). -->
          <span></span>
        </div>
      {/each}
    {/if}
  </div>
{:else}
  <div class="border-t border-divider bg-control/30 px-4 py-2.5 lg:pl-[94px]">
    {#if rows.length === 0}
      <p class="font-mono text-[12px] text-subtle">
        {loaded ? ($locale && m.files_waiting_metadata()) : "…"}
      </p>
    {:else}
      <!-- CARD no mobile, LINHA no desktop, de uma definição só. Em telas estreitas a linha
           única quebrava em três pedaços soltos (nome, depois o chip do codec sozinho, depois
           barra e tamanho), sem nada agrupando o que era de qual arquivo — o `flex-wrap` não
           tem como saber onde termina um item. O card resolve com a borda.
           `lg:contents` some com o wrapper do rodapé no desktop, e aí os filhos dele voltam a
           ser itens do mesmo flex do nome — uma linha só, sem markup duplicado. -->
      <ul class="space-y-2 lg:space-y-1.5">
        {#each rows as f (f.path)}
          {@const pct = progressOf(f)}
          <li
            class="rounded-field border border-default bg-surface p-2.5 lg:flex lg:items-center lg:gap-x-3
                   lg:rounded-none lg:border-0 lg:bg-transparent lg:p-0"
          >
            <!-- O nome NÃO é `flex-1`: com ele o nome comia a linha inteira e empurrava o chip
                 do codec para a borda direita, longe do arquivo a que se refere. Encolhendo
                 pelo conteúdo, o chip fica colado no nome, e o `flex-1` vai para o espaçador. -->
            <span class="block min-w-0 truncate font-mono text-[12px] text-body" title={f.path}>
              {#if mode === "episodes"}
                {$locale && m.detail_ep_title({ number: f.episode ?? 0 })}
              {:else}
                {baseName(f.path)}
              {/if}
            </span>
            <div class="mt-2 flex items-center gap-2 lg:contents">
              <!-- Codec só no modo `raw` (Downloads): é a resposta para "em que formato veio", e
                   a tela do anime é sobre episódio. Só aparece quando o backend conseguiu ler o
                   cabeçalho — arquivo em voo ou fora do mkv não ganha chip nenhum, em vez de um
                   "desconhecido" ocupando a linha. -->
              {#if mode === "raw" && f.codec}
                <span class="shrink-0"><Chip variant="neutral">{f.codec}</Chip></span>
              {/if}
              <span class="hidden lg:block lg:flex-1"></span>
              <span class="min-w-0 flex-1 lg:w-[104px] lg:flex-none">
                <ProgressBar value={pct ?? 0} thickness={4} variant={pct === 1 ? "ok" : "accent"} label={f.path} />
              </span>
              <span class="shrink-0 font-mono text-[12px] text-subtle">{detailsOf(f)}</span>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
