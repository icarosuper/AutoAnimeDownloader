<script lang="ts">
  // TripleProgressBar — spec §6 (Fase 2). Three ADJACENT, SUMMED segments over one track (not
  // overlaid): watched (--ok), downloaded (--accent), released (--warn), scaled against
  // `total`. Each segment's width is the DELTA from the previous one.
  //
  // As props sao CUMULATIVAS (watched <= downloaded <= released <= total) e NAO sao os campos
  // crus de `AnimeInfo`: `downloaded` precisa vir de `breakdown()` (lib/utils/status.ts), que
  // o deriva de `episodes_pending`. Passar `episodes_downloaded` aqui pinta de amarelo
  // episodio ja baixado e visto, porque aquele campo cai quando o daemon poda assistidos —
  // decisions.md #88.
  //
  // A legenda mora AQUI, e nao numa prop `legend: string` como antes, porque cada termo dela e
  // colorido com a cor do seu segmento — e uma string pronta nao tem onde pendurar o <span>.
  // Os quatro termos sao os DELTAS (o que cada faixa de cor mede), nao os cumulativos: a
  // legenda antiga dizia "5 vistos · 5 baixados" enquanto o roxo tinha largura zero, e era
  // impossivel casar palavra com cor. Aqui os mesmos quatro numeros alimentam largura e texto,
  // entao eles nao tem como divergir de novo.
  import * as m from "../../lib/i18n/messages.js"
  import { locale } from "../../lib/stores/locale.js"

  export let watched: number
  export let downloaded: number
  export let released: number
  export let total: number
  // Por anime, termo zerado e ruido: a maioria das linhas tem pelo menos um, e a legenda de
  // quatro termos e larga o bastante para quebrar em duas linhas na coluna de progresso. No
  // resumo da biblioteca e o contrario — la os quatro numeros sao o conteudo do card, e um
  // termo sumindo faria a soma parar de fechar com o total. Por isso o default omite.
  export let keepZeros = false

  $: toWatch = Math.max(0, downloaded - watched)
  $: toDownload = Math.max(0, released - downloaded)
  $: unreleased = Math.max(0, total - released)

  function pct(n: number): number {
    if (!total || total <= 0) return 0
    return Math.max(0, Math.min(100, (n / total) * 100))
  }

  // `text-subtle` do <p> ja cobre "nao lancados" — a faixa dele e o proprio track, sem cor.
  $: allTerms = $locale
    ? [
        { n: watched, cls: "text-ok", text: m.progress_legend_watched({ count: watched }) },
        { n: toWatch, cls: "text-accent", text: m.progress_legend_to_watch({ count: toWatch }) },
        { n: toDownload, cls: "text-warn", text: m.progress_legend_to_download({ count: toDownload }) },
        { n: unreleased, cls: "", text: m.progress_legend_unreleased({ count: unreleased }) },
      ]
    : []
  $: kept = keepZeros ? allTerms : allTerms.filter((t) => t.n > 0)
  // Anime sem episodio lancado zera os quatro termos, e a legenda inteira sumiria — <p> vazio
  // encolhe a linha e faz a altura oscilar, justo o que a coluna de progresso nao pode ter.
  // Nesse caso sobra "0 vistos", que e verdade e ocupa a linha.
  $: terms = kept.length > 0 ? kept : allTerms.slice(0, 1)
</script>

<div class="w-full">
  <!-- Decorative: the accessible description is the visible legend paragraph below, not this
       row of colored segments. -->
  <div class="flex w-full overflow-hidden rounded-pill bg-track" style="height:9px" aria-hidden="true">
    <div class="h-full bg-ok" style="width:{pct(watched)}%"></div>
    <div class="h-full bg-accent" style="width:{pct(toWatch)}%"></div>
    <div class="h-full bg-warn" style="width:{pct(toDownload)}%"></div>
  </div>
  <p class="mt-1.5 text-caption text-subtle">
    {#each terms as term, i}{#if i > 0}<span aria-hidden="true">{' · '}</span>{/if}<span
        class={term.cls}>{term.text}</span
      >{/each}
  </p>
</div>
