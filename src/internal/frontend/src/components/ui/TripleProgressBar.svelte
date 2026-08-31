<script lang="ts">
  // TripleProgressBar — spec §6 (Fase 2). Three ADJACENT, SUMMED segments over one track (not
  // overlaid): watched (--ok), downloaded (--accent), released (--warn), scaled against
  // `total`. Each segment's width is the DELTA from the previous one, so the bar reads
  // left-to-right as "this much watched, then this much more downloaded-but-unwatched, then
  // this much more released-but-undownloaded".
  //
  // As props sao CUMULATIVAS (watched <= downloaded <= released <= total) e NAO sao os campos
  // crus de `AnimeInfo`: `downloaded` precisa vir de `breakdown()` (lib/utils/status.ts), que
  // o deriva de `episodes_pending`. Passar `episodes_downloaded` aqui pinta de amarelo
  // episodio ja baixado e visto, porque aquele campo cai quando o daemon poda assistidos.
  //
  // `legend` is required (no default value) on purpose — spec §6 is explicit that this bar is
  // never shown without a textual legend, because three stacked colors alone don't communicate
  // actual numbers. Making it optional would make it easy to forget; the compiler now expects
  // it wherever this component is used.
  export let watched: number
  export let downloaded: number
  export let released: number
  export let total: number
  export let legend: string

  function pctOf(n: number): number {
    if (!total || total <= 0) return 0
    return Math.max(0, Math.min(100, (n / total) * 100))
  }

  $: watchedPct = pctOf(watched)
  $: downloadedPct = Math.max(0, pctOf(downloaded) - watchedPct)
  $: releasedPct = Math.max(0, pctOf(released) - watchedPct - downloadedPct)
</script>

<div class="w-full">
  <!-- Decorative: the accessible description is the visible legend paragraph below, not this
       row of colored segments. -->
  <div class="flex w-full overflow-hidden rounded-pill bg-track" style="height:9px" aria-hidden="true">
    <div class="h-full bg-ok" style="width:{watchedPct}%"></div>
    <div class="h-full bg-accent" style="width:{downloadedPct}%"></div>
    <div class="h-full bg-warn" style="width:{releasedPct}%"></div>
  </div>
  <p class="mt-1.5 text-caption text-subtle">{legend}</p>
</div>
