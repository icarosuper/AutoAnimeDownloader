<script lang="ts">
  // TripleProgressBar — spec §6 (Fase 2). Three ADJACENT, SUMMED segments over one track (not
  // overlaid): watched (--ok), downloaded (--accent), released (--warn), scaled against
  // `total`. Each segment's width is the DELTA from the previous one.
  //
  // The props are CUMULATIVE (watched <= downloaded <= released <= total) and are NOT the raw
  // `AnimeInfo` fields: `downloaded` must come from `breakdown()` (lib/utils/status.ts), which
  // derives it from `episodes_pending`. Passing `episodes_downloaded` here paints already
  // downloaded-and-watched episodes yellow, because that field drops when the daemon prunes
  // watched ones — decisions.md #88.
  //
  // The legend lives HERE, not in a `legend: string` prop like before, because each of its
  // terms is colored with its segment's color — and a ready-made string has nowhere to hang
  // the <span>. The four terms are the DELTAS (what each color band measures), not the
  // cumulatives: the old legend said "5 watched · 5 downloaded" while purple had zero width,
  // and it was impossible to match word to color. Here the same four numbers feed width and
  // text, so they can't diverge again.
  import * as m from "../../lib/i18n/messages.js"
  import { locale } from "../../lib/stores/locale.js"

  export let watched: number
  export let downloaded: number
  export let released: number
  export let total: number
  // Per anime, a zero term is noise: most rows have at least one, and a four-term legend is
  // wide enough to wrap onto two lines in the progress column. In the library summary it's
  // the opposite — there the four numbers ARE the card's content, and a term vanishing would
  // make the sum stop adding up to the total. That's why the default omits.
  export let keepZeros = false

  $: toWatch = Math.max(0, downloaded - watched)
  $: toDownload = Math.max(0, released - downloaded)
  $: unreleased = Math.max(0, total - released)

  function pct(n: number): number {
    if (!total || total <= 0) return 0
    return Math.max(0, Math.min(100, (n / total) * 100))
  }

  // The <p>'s `text-subtle` already covers "unreleased" — its band is the track itself, with no color.
  $: allTerms = $locale
    ? [
        { n: watched, cls: "text-ok", text: m.progress_legend_watched({ count: watched }) },
        { n: toWatch, cls: "text-accent", text: m.progress_legend_to_watch({ count: toWatch }) },
        { n: toDownload, cls: "text-warn", text: m.progress_legend_to_download({ count: toDownload }) },
        { n: unreleased, cls: "", text: m.progress_legend_unreleased({ count: unreleased }) },
      ]
    : []
  $: kept = keepZeros ? allTerms : allTerms.filter((t) => t.n > 0)
  // An anime with no released episode zeros all four terms, and the whole legend would
  // vanish — an empty <p> shrinks the row and makes the height oscillate, exactly what the
  // progress column cannot have. In that case "0 watched" remains, which is true and fills
  // the row.
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
