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

  $: toWatch = Math.max(0, downloaded - watched)
  $: toDownload = Math.max(0, released - downloaded)
  $: unreleased = Math.max(0, total - released)

  function pct(n: number): number {
    if (!total || total <= 0) return 0
    return Math.max(0, Math.min(100, (n / total) * 100))
  }
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
    <span class="text-ok">{$locale && m.progress_legend_watched({ count: watched })}</span>
    <span aria-hidden="true"> · </span>
    <span class="text-accent">{$locale && m.progress_legend_to_watch({ count: toWatch })}</span>
    <span aria-hidden="true"> · </span>
    <span class="text-warn">{$locale && m.progress_legend_to_download({ count: toDownload })}</span>
    <span aria-hidden="true"> · </span>
    <span>{$locale && m.progress_legend_unreleased({ count: unreleased })}</span>
  </p>
</div>
