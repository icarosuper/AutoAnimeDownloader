<script lang="ts">
  // Banner de sistema — UM só, com precedência, hospedado no AppShell (spec: decisions.md #66).
  // Não é toast: toast é falha de uma AÇÃO que o usuário pediu e some sozinho; banner é estado
  // DEGRADADO que persiste entre polls e explica por que a tela inteira parece errada. O valor
  // dele não é reportar o erro — é dizer por que a lista está velha ou incompleta, coisa que
  // hoje o usuário conclui sozinho (errado) como "sumiu anime".
  import { onDestroy, onMount } from 'svelte'
  import { AlertTriangle, ExternalLink } from '@lucide/svelte'
  import { getStatus, type AnilistHealth } from '../../lib/api/client.js'
  import { backendHealth } from '../../lib/stores/backendHealth.js'
  import { pickBanner, secondsUntil } from '../../lib/domain/systemBanner.js'
  import { locale } from '../../lib/stores/locale.js'
  import * as m from '../../lib/i18n/messages.js'

  const REPO_ISSUES = 'https://github.com/icarosuper/AutoAnimeDownloader/issues/new'
  // Poll só para alimentar o banner. Devagar de propósito: o estado que ele mostra muda em
  // escala de minutos (outage, timeout de 1 min do rate limit), não de segundos.
  const STATUS_POLL_MS = 30000

  let anilist: AnilistHealth | null = null
  let now = Date.now()
  let pollId: ReturnType<typeof setInterval> | null = null
  let tickId: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      // silent: uma falha aqui já é contada pelo backendHealth e vira o próprio banner. Um
      // toast a cada 30s com o daemon fora do ar seria só barulho.
      anilist = (await getStatus({ silent: true })).anilist ?? null
    } catch {
      // O estado da AniList fica com o último valor conhecido; a precedência do banner garante
      // que ele não seja mostrado enquanto o backend estiver fora — seria informação datada.
    }
  }

  onMount(() => {
    loadStatus()
    pollId = setInterval(loadStatus, STATUS_POLL_MS)
    // Segundo timer só para a contagem regressiva do rate limit, que é o único caso em que se
    // sabe o tempo exato que falta (Retry-After).
    tickId = setInterval(() => (now = Date.now()), 1000)
  })

  onDestroy(() => {
    if (pollId) clearInterval(pollId)
    if (tickId) clearInterval(tickId)
  })

  $: banner = pickBanner($backendHealth, anilist)
  $: seconds = banner ? secondsUntil(banner.retryAt, now) : 0

  $: text =
    $locale && banner
      ? banner.kind === 'backend_unreachable'
        ? m.banner_backend_unreachable()
        : banner.kind === 'backend_error'
          ? m.banner_backend_error()
          : banner.kind === 'anilist_outage'
            ? m.banner_anilist_outage()
            : banner.kind === 'anilist_app_bug'
              ? m.banner_anilist_app_bug()
              : seconds > 0
                ? m.banner_anilist_rate_limited({ seconds })
                : m.banner_anilist_rate_limited_no_eta()
      : ''

  // Título e corpo pré-preenchidos: o report só vale se trouxer o que só o usuário tem.
  $: reportHref = banner
    ? `${REPO_ISSUES}?title=${encodeURIComponent(`[${banner.kind}] `)}&body=${encodeURIComponent(
        `**O que aconteceu:**\n\n\n---\nBanner: \`${banner.kind}\`\nDetalhe: \`${banner.detail ?? '-'}\`\n`,
      )}`
    : ''
</script>

{#if banner}
  <!-- role="alert" e não "status": o conteúdo abaixo do banner está errado ou parado, e o
       leitor de tela precisa saber disso antes de continuar lendo a página. -->
  <div
    role="alert"
    data-testid="system-banner"
    data-banner-kind={banner.kind}
    class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-4 py-2.5 text-copy sm:px-6 lg:px-8 {banner.reportable
      ? 'border-danger-tint/32 bg-danger-tint/12 text-danger'
      : 'border-warn-tint/32 bg-warn-tint/12 text-warn'}"
  >
    <AlertTriangle class="h-4 w-4 shrink-0" aria-hidden="true" />
    <span class="min-w-0 flex-1">
      {text}
      {#if banner.detail}
        <!-- Mensagem crua da AniList. Um 403 de IP bloqueado explica o motivo por escrito e é a
             única informação que o frontend não tem como reconstruir — mostrar verbatim. -->
        <span class="block text-caption opacity-80">{banner.detail}</span>
      {/if}
    </span>
    {#if banner.reportable}
      <a
        href={reportHref}
        target="_blank"
        rel="noopener noreferrer"
        class="inline-flex shrink-0 items-center gap-1 underline underline-offset-2 hover:opacity-80"
      >
        {$locale && m.banner_report()}
        <ExternalLink class="h-3.5 w-3.5" aria-hidden="true" />
      </a>
    {/if}
  </div>
{/if}
