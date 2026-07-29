<script lang="ts">
  // Cover — spec §6 (Fase 2). AniList `cover_image`, with a hatched fallback (repeating-linear
  // -gradient) for animes that don't have one — replaces the "no cover, just blank space" gap
  // in the pre-redesign screens (AnimeDetail.svelte only renders an <img> when `cover_image`
  // is present, nothing otherwise).
  export let src: string | undefined = undefined
  export let alt = ''
  /** Border radius Tailwind key (spec §4.2). Covers appear in different contexts (list row vs. hero), hence a prop instead of a fixed value. */
  export let radiusClass: 'rounded-field' | 'rounded-card' = 'rounded-field'
</script>

{#if src}
  <img {src} {alt} class="h-full w-full object-cover {radiusClass}" />
{:else}
  <div
    class="h-full w-full {radiusClass}"
    style="background-color: var(--bg-sunken); background-image: repeating-linear-gradient(45deg, var(--border-default) 0, var(--border-default) 1px, transparent 1px, transparent 7px)"
    role="img"
    aria-label={alt}
  ></div>
{/if}
