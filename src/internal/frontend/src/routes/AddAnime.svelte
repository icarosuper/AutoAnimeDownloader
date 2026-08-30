<script lang="ts">
  // AddAnime — tela de adicionar anime avulso (spec "Download de animes avulsos", §Frontend).
  //
  // Rota, e não modal: é linkável, recarregável e uma grade de capas cabe melhor no mobile que
  // um modal. A gestão dos avulsos já existentes vive no Status, então esta tela tem um
  // trabalho só — achar um anime e passar a acompanhá-lo.
  import { onDestroy, onMount } from "svelte";
  import { ArrowRight, ExternalLink, Search } from "@lucide/svelte";
  import {
    searchAniList,
    addStandaloneAnime,
    getConfig,
    ApiError,
    type AniListSearchResult,
    type BlockReason,
  } from "../lib/api/client.js";
  import Button from "../components/ui/Button.svelte";
  import Cover from "../components/ui/Cover.svelte";
  import Loading from "../components/Loading.svelte";
  import Toggle from "../components/ui/Toggle.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // A busca sai no submit (botão ou Enter), NÃO a cada tecla: o limite da AniList é de 30 req/min
  // por IP e a busca é PriorityDisposable, então quem digita devagar queimava o balde numa
  // requisição por tecla e via a própria busca seguinte ser recusada pelo gate de orçamento.
  // Um submit = uma requisição. O AbortController continua porque dois submits em sequência
  // ainda correm entre si, e a resposta velha não pode pintar por cima da nova.
  const MIN_TERM = 3;
  const ANILIST_URL = "https://anilist.co/anime/";

  let term = "";
  /** Desligado por padrão: quem chega aqui quer baixar algo agora, e o que não estreou não baixa. */
  let includeUnreleased = false;
  let results: AniListSearchResult[] = [];
  let searching = false;
  let searchFailed = false;
  let searched = false;
  /** media ids em voo ou já adicionados nesta sessão — o botão do card sai do estado "Adicionar". */
  let adding = new Set<number>();
  let added = new Set<number>();
  let libraryConfigured = true;

  let inFlight: AbortController | undefined;

  onMount(async () => {
    try {
      const config = await getConfig();
      libraryConfigured = Boolean(config.completed_anime_path);
    } catch {
      // Sem a config não dá para afirmar que a biblioteca falta; o POST recusa com
      // LIBRARY_NOT_CONFIGURED de qualquer forma.
    }
  });

  onDestroy(() => inFlight?.abort());

  // O toggle só refaz a busca quando já existe uma na tela: ele muda o filtro de um resultado
  // visível. Sem busca feita ainda, ele apenas escolhe o parâmetro do próximo submit.
  function onToggleUnreleased(): void {
    if (searched) runSearch();
  }

  async function runSearch(): Promise<void> {
    const q = term.trim();
    inFlight?.abort();

    if (q.length < MIN_TERM) {
      results = [];
      searched = false;
      searchFailed = false;
      searching = false;
      return;
    }

    const controller = new AbortController();
    inFlight = controller;
    searching = true;
    searchFailed = false;

    try {
      const found = await searchAniList(q, includeUnreleased, controller.signal);
      if (controller.signal.aborted) return;
      results = found;
      searched = true;
    } catch (error) {
      if (controller.signal.aborted || (error instanceof Error && error.name === "AbortError")) return;
      searchFailed = true;
      results = [];
    } finally {
      if (!controller.signal.aborted) searching = false;
    }
  }

  // O front é best-effort e o backend é a autoridade: block_reason vem de um snapshot com até
  // 60s de idade, então um card pode estar clicável para um anime que acabou de entrar na
  // lista. O 409 traduzido abaixo é a palavra final; não há retry nem revalidação, a próxima
  // busca já vem correta.
  async function add(result: AniListSearchResult): Promise<void> {
    adding = new Set(adding).add(result.id);
    try {
      const { added: count } = await addStandaloneAnime(result.id);
      added = new Set(added).add(result.id);
      if (count > 0) {
        toast.success(m.add_toast_added({ count }), undefined, {
          href: "#/downloads",
          label: m.add_toast_downloads_link(),
        });
      } else {
        // Um "0 episódios adicionados" seco leria como falha no caso mais comum: anime que
        // ainda vai estrear.
        toast.info(m.add_toast_tracked_no_episodes());
      }
    } catch (error) {
      toast.error(addErrorMessage(error));
    } finally {
      const next = new Set(adding);
      next.delete(result.id);
      adding = next;
    }
  }

  function addErrorMessage(error: unknown): string {
    if (!(error instanceof ApiError)) {
      return error instanceof Error ? error.message : m.add_search_error();
    }
    switch (error.code) {
      case "LIBRARY_NOT_CONFIGURED":
        return m.block_library_not_configured();
      case "ALREADY_BLACKLISTED":
        return m.block_blacklist();
      case "ALREADY_TRACKED":
        return m.block_tracked();
      case "ALREADY_DOWNLOADED":
        return m.block_downloaded();
      case "ALREADY_STANDALONE":
        return m.block_standalone();
      default:
        return error.message;
    }
  }

  function blockLabel(reason: BlockReason): string {
    switch (reason) {
      case "blacklist":
        return m.block_blacklist();
      case "tracked":
        return m.block_tracked();
      case "downloaded":
        return m.block_downloaded();
      default:
        return "";
    }
  }

  function metaLine(result: AniListSearchResult): string {
    const parts: string[] = [];
    if (result.format) parts.push(result.format.replace(/_/g, " "));
    if (result.year) parts.push(String(result.year));
    if (result.episodes) parts.push(m.add_meta_episodes({ count: result.episodes }));
    if (result.status) parts.push(result.status.replace(/_/g, " "));
    return parts.join(" · ");
  }

  // Um card só termina em botão morto quando não há para onde ir. "standalone", "tracked" e
  // "downloaded" significam que o anime JÁ EXISTE no app — e um anime que existe tem página em
  // #/status/{id}, porque anime_id é o próprio media id do AniList. Nesses três o botão vira o
  // link para lá; o motivo, que antes vivia só num tooltip (invisível no mobile), passa a ser
  // uma linha do card.
  //
  // "blacklist" é a única exceção: está numa lista excluída, o daemon não o processa e não há
  // detalhe para abrir. Só ele continua apagado e com o botão desabilitado.
  $: cards =
    $locale &&
    results.map((result) => {
      const isAdded = added.has(result.id) || result.block_reason === "standalone";
      const blacklisted = result.block_reason === "blacklist";
      const hasStatusPage =
        isAdded || result.block_reason === "tracked" || result.block_reason === "downloaded";
      return {
        result,
        meta: metaLine(result),
        note: isAdded ? m.block_standalone() : blockLabel(result.block_reason),
        isAdding: adding.has(result.id),
        blacklisted,
        statusHref: hasStatusPage ? `#/status/${result.id}` : undefined,
        anilistHref: `${ANILIST_URL}${result.id}`,
      };
    });
</script>

<div class="space-y-4.5">
  <div>
    <h1 class="text-screen-title text-heading">{$locale && m.add_title()}</h1>
    <p class="mt-0.5 text-caption text-subtle">{$locale && m.add_subtitle()}</p>
  </div>

  {#if !libraryConfigured}
    <div
      role="alert"
      class="flex flex-wrap items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
    >
      {$locale && m.add_library_required()}
      <a href="#/config" class="underline">{$locale && m.nav_config()}</a>
    </div>
  {/if}

  <!-- <form>, e não um on:keydown no input: o Enter que dispara o submit é comportamento nativo
       do formulário, e o botão participa dele só por ser type="submit". -->
  <form class="flex items-center gap-2" on:submit|preventDefault={runSearch}>
    <label
      class="flex flex-1 items-center gap-2 rounded-field border border-default bg-control px-2.5 py-2 focus-within:border-accent"
    >
      <Search size={15} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
      <input
        type="search"
        bind:value={term}
        placeholder={$locale && m.add_search_placeholder()}
        aria-label={$locale && m.add_search_placeholder()}
        class="w-full bg-transparent text-copy text-heading outline-none placeholder:font-normal placeholder:text-subtle"
      />
    </label>
    <Button type="submit" disabled={term.trim().length < MIN_TERM || searching}>
      {$locale && m.add_btn_search()}
    </Button>
  </form>

  <Toggle
    id="add-include-unreleased"
    bind:checked={includeUnreleased}
    on:change={onToggleUnreleased}
    label={($locale && m.add_toggle_unreleased()) || ""}
  />

  {#if searching}
    <Loading message={$locale && m.add_search_placeholder()} />
  {:else if searchFailed}
    <p role="alert" class="text-copy text-danger">{$locale && m.add_search_error()}</p>
  {:else if !searched}
    <p class="text-copy text-subtle">{$locale && m.add_search_hint()}</p>
  {:else if searched && results.length === 0}
    <p class="text-copy text-subtle">{$locale && m.add_no_results()}</p>
  {:else}
    <ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {#each cards || [] as card (card.result.id)}
        <li
          class="flex gap-3 rounded-card border border-default bg-card p-3 {card.blacklisted
            ? 'opacity-50'
            : ''}"
        >
          <div class="h-[92px] w-[64px] shrink-0 overflow-hidden">
            <Cover src={card.result.cover} alt={card.result.title} radiusClass="rounded-field" />
          </div>
          <div class="flex min-w-0 flex-1 flex-col gap-1.5">
            <a
              href={card.anilistHref}
              target="_blank"
              rel="noopener noreferrer"
              title={card.result.title}
              aria-label="{card.result.title} — {$locale && m.add_link_anilist()}"
              class="flex items-center gap-1 hover:underline"
            >
              <span class="truncate text-copy text-heading">{card.result.title}</span>
              <ExternalLink size={12} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
            </a>
            <p class="truncate font-mono text-caption text-subtle">{card.meta}</p>
            {#if card.note}
              <p class="truncate text-caption text-subtle">{card.note}</p>
            {/if}
            <div class="mt-auto self-start">
              {#if card.statusHref}
                <Button variant="ghost" href={card.statusHref}>
                  {$locale && m.add_btn_view_status()}
                  <ArrowRight size={13} strokeWidth={2} aria-hidden="true" />
                </Button>
              {:else}
                <Button
                  variant="solid"
                  disabled={card.blacklisted || card.isAdding}
                  on:click={() => add(card.result)}
                >
                  {#if card.isAdding}
                    {$locale && m.add_btn_adding()}
                  {:else}
                    {$locale && m.add_btn_add()}
                  {/if}
                </Button>
              {/if}
            </div>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
