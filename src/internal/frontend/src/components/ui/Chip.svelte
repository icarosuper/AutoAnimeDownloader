<script lang="ts">
  // Chip — spec §6 (Fase 2). Tinted background + tinted border, one of five status variants.
  // `deriveAnimeChip` (lib/domain/animeState.ts) returns a `variant` that maps 1:1 onto this
  // prop; screens translate the chip's `key` into the slotted text themselves (this component
  // never sees a `key`, only rendered content via the slot — keeps it free of any i18n
  // concern, same boundary as the domain module it pairs with).
  //
  // Opacity follows spec §4.1: tinted backgrounds 7-16%, tinted borders 22-32%. `neutral` has
  // no dedicated "-tint" token (see tailwind.config.js semanticColors) — it uses its own solid
  // color at the same opacities, which is the same visual effect the tint tokens produce for
  // the other four variants (tint === the base color for aad-dark; only aad-light splits them).
  export let variant: 'accent' | 'ok' | 'warn' | 'danger' | 'neutral' = 'neutral'
  // "linha esmaecida" (spec §7, blacklisted branch of deriveAnimeChip) — dims the whole chip,
  // not just the text, so it reads as de-emphasized at a glance in a list.
  export let dimmed = false

  const VARIANT_CLASSES: Record<string, string> = {
    accent: 'bg-accent-tint/12 border-accent-tint/28 text-accent',
    ok: 'bg-ok-tint/12 border-ok-tint/28 text-ok',
    warn: 'bg-warn-tint/12 border-warn-tint/28 text-warn',
    danger: 'bg-danger-tint/12 border-danger-tint/28 text-danger',
    neutral: 'bg-neutral/12 border-neutral/28 text-neutral',
  }

  $: classes = VARIANT_CLASSES[variant] ?? VARIANT_CLASSES.neutral
</script>

<span
  class="inline-flex items-center gap-1 rounded-badge border px-2 py-0.5 text-caption font-semibold leading-none {classes} {dimmed
    ? 'opacity-60'
    : ''}"
>
  <slot />
</span>
