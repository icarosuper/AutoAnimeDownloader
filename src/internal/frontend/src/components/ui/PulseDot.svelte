<script lang="ts">
  // PulseDot — spec §6 (Fase 2). 2.4s ease-in-out infinite keyframe animation, opacity 1->.35,
  // scale 1->.82. Spec is explicit this is ONLY for "alive"/live indicators (e.g. daemon
  // running, WebSocket connected) — do not reach for it as generic decoration. Purely
  // decorative (`aria-hidden`): whatever text conveys "live" ("Ao vivo", "Rodando", ...) lives
  // next to this dot in the consuming screen, same as `NavRail`'s existing WS status dot.
  export let variant: 'accent' | 'ok' | 'warn' | 'danger' | 'neutral' = 'ok'
  export let size = 8 // px

  const VARIANT_CLASSES: Record<string, string> = {
    accent: 'bg-accent',
    ok: 'bg-ok',
    warn: 'bg-warn',
    danger: 'bg-danger',
    neutral: 'bg-neutral',
  }

  $: dotClass = VARIANT_CLASSES[variant] ?? VARIANT_CLASSES.ok
</script>

<span
  class="pulse-dot inline-block shrink-0 rounded-full {dotClass}"
  style="width:{size}px; height:{size}px"
  aria-hidden="true"
></span>

<style>
  .pulse-dot {
    animation: pulse-dot 2.4s ease-in-out infinite;
  }

  @keyframes pulse-dot {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.35;
      transform: scale(0.82);
    }
  }
</style>
