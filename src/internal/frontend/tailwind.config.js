import daisyui from 'daisyui'

/*
 * Cores semânticas (fase 0 do redesign de UI). Fonte da verdade: src/lib/design/tokens.css.
 *
 * Cada entrada abaixo usa `rgb(var(--x-rgb) / <alpha-value>)` (não um hex fixo) para as
 * cores de acento/status, o que habilita o modificador de opacidade do Tailwind
 * (`bg-ok/12`, `border-ok-tint/28`, ...). Superfícies e texto não precisam de opacidade
 * variável, então usam `var(--x)` direto.
 *
 * `heading`/`body` correspondem a --text-primary/--text-secondary do tokens.css. Não usamos
 * os nomes `primary`/`secondary` como chave do Tailwind porque o daisyUI já reserva esses
 * dois nomes para sua própria paleta de tema (--p/--s, cor de marca), e ambos os plugins
 * gerariam a mesma classe `.text-primary`/`.text-secondary` com significados diferentes.
 * `tertiary` e `subtle` não colidem com o vocabulário do daisyUI e mantêm o nome do token.
 */
const semanticColors = {
  // fundos
  window: 'var(--bg-window)',
  surface: 'var(--bg-surface)',
  card: 'var(--bg-card)',
  sunken: 'var(--bg-sunken)',
  control: 'var(--bg-control)',
  menu: 'var(--bg-menu)',

  // bordas (já são rgba() no token, não precisam de modificador de opacidade do Tailwind)
  default: 'var(--border-default)',
  divider: 'var(--border-divider)',

  // texto — ver nota acima sobre o nome heading/body
  heading: 'var(--text-primary)',
  body: 'var(--text-secondary)',
  tertiary: 'var(--text-tertiary)',
  subtle: 'var(--text-subtle)',

  // acento e status — texto/borda/fill sólido
  accent: 'rgb(var(--accent-rgb) / <alpha-value>)',
  ok: 'rgb(var(--ok-rgb) / <alpha-value>)',
  warn: 'rgb(var(--warn-rgb) / <alpha-value>)',
  danger: 'rgb(var(--danger-rgb) / <alpha-value>)',
  neutral: 'rgb(var(--neutral-rgb) / <alpha-value>)',

  // variantes para fundos/bordas tingidos (7–16% / 22–32% de opacidade — ver tokens.css)
  'accent-tint': 'rgb(var(--accent-tint-rgb) / <alpha-value>)',
  'ok-tint': 'rgb(var(--ok-tint-rgb) / <alpha-value>)',
  'warn-tint': 'rgb(var(--warn-tint-rgb) / <alpha-value>)',
  'danger-tint': 'rgb(var(--danger-tint-rgb) / <alpha-value>)',

  // texto sobre acento/status sólido
  'on-accent': 'var(--on-accent)',
  'on-ok': 'var(--on-ok)',
  'on-warn': 'var(--on-warn)',
  'on-danger': 'var(--on-danger)',

  // trilho de barra de progresso (§4.2)
  track: 'var(--progress-track)',

  // texto do item ativo do rail/tab bar/MoreMenu (§5, Fase 1) — token porque o spec fixa um
  // hex literal (#c5bcfa) só para o tema escuro; ver tokens.css.
  'nav-active': 'var(--nav-active-text)',
}

/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  darkMode: ['class', '[data-theme="aad-dark"]'],
  theme: {
    extend: {
      colors: semanticColors,
      fontFamily: {
        sans: ['Manrope', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      // §4.2 — espaçamento: preenche os furos 18/22/26px na escala padrão do Tailwind
      // (que já cobre 4/6/8/10/14px via as chaves 1/1.5/2/2.5/3.5).
      spacing: {
        4.5: '18px',
        5.5: '22px',
        6.5: '26px',
      },
      // §4.2 — raios. Nomes próprios (não sobrescrevem md/lg/xl/2xl) para não afetar telas
      // que ainda usam os raios padrão do Tailwind antes de serem redesenhadas.
      borderRadius: {
        badge: '6px',
        control: '9px',
        field: '12px',
        card: '16px',
        modal: '18px',
        pill: '999px',
      },
      // §4.2 — elevação (modal, menu), varia por tema via tokens.css.
      boxShadow: {
        elevation: 'var(--elevation-shadow)',
      },
      // §4.3 — escala tipográfica. Registrada aqui como utilitário `text-*`; nenhuma tela
      // consome isso ainda (fica para as fases seguintes). O spec dá faixas, não valores
      // únicos — usamos o limite indicado no comentário de cada entrada. `line-height` não
      // é dado pelo spec, então é deixado de fora (comportamento padrão do Tailwind) em vez
      // de inventar um valor. `rótulo mono` também pede maiúsculo — isso é `uppercase`
      // aplicado pelo consumidor junto com `text-mono-label`, não faz parte do tuple de
      // font-size do Tailwind.
      fontSize: {
        // título de tela: 20–22px/800, -.01em
        'screen-title': ['22px', { letterSpacing: '-0.01em', fontWeight: '800' }],
        // número herói: 46px/600, -.03em
        'hero-number': ['46px', { letterSpacing: '-0.03em', fontWeight: '600' }],
        // número de card: 22–30px/650 (650 é literal do spec; não há peso 650 carregado —
        // o navegador casa para o peso mais próximo disponível)
        'card-number': ['30px', { fontWeight: '650' }],
        // título de card: 14–15px/700
        'card-title': ['15px', { fontWeight: '700' }],
        // corpo: 13–13.5px/600
        copy: ['13.5px', { fontWeight: '600' }],
        // secundário: 12–12.5px/500
        caption: ['12.5px', { fontWeight: '500' }],
        // rótulo mono: 10.5–11px/600, letter-spacing .10–.12em, maiúsculo
        'mono-label': ['11px', { letterSpacing: '0.12em', fontWeight: '600' }],
      },
    },
  },
  plugins: [daisyui],
  daisyui: {
    themes: [
      {
        'aad-dark': {
          'color-scheme': 'dark',
          // acento único do app: primary/secondary/accent do daisyUI apontam todos para o
          // mesmo roxo — a regra de composição do §4.1 permite no máximo um acento sólido
          // por tela, então não existe um segundo "secondary"/"accent" de verdade aqui.
          primary: '#8272ee',
          'primary-content': '#12111a',
          secondary: '#8272ee',
          'secondary-content': '#12111a',
          accent: '#8272ee',
          'accent-content': '#12111a',
          // neutral: token --neutral do tokens.css (cor de status "sem estado"), não uma
          // superfície estrutural — usado hoje só no botão flutuante "voltar ao topo" de Logs.
          neutral: '#8d93a3',
          'neutral-content': '#12111a',
          'base-100': '#101218', // = --bg-card
          'base-200': '#0a0c11', // = --bg-sunken
          // base-300 não tem um token dedicado no spec. CORRIGIDO na rodada de fix 1: a
          // primeira versão (#1b1c1f) tratava base-300 como se fosse usado majoritariamente
          // como borda e o "achatava" a partir de --border-default sobre --bg-window — isso
          // ficava MAIS CLARO que base-100/200, invertendo a progressão de escurecimento do
          // daisyUI. Levantamento real (`grep -rn "bg-base-300" src`) mostra 5 usos como
          // FUNDO sólido (não borda): chip de filtro inativo e swatch da legenda
          // (Status.svelte:380,453), trilho de barra de progresso (Status.svelte:533,613) e
          // painel do visualizador de logs (Logs.svelte:282) — todos pedem a superfície mais
          // recuada/escura da hierarquia, não uma linha clara. Valor abaixo = base-200
          // escurecido 7% em direção ao preto em OKLCH (mesma fórmula que o próprio daisyUI
          // usa para auto-gerar base-300 quando ele não é informado — ver
          // node_modules/daisyui/src/theming/functions.js:generateDarkenColorFrom), mantendo
          // a progressão monotônica base-100 > base-200 > base-300 igual ao tema `dark`
          // padrão do daisyUI e ao nosso próprio tema claro.
          'base-300': '#080a0e',
          'base-content': '#e8eaf0',
          // info não tem token próprio no spec; reaproveita o acento (mesma família de cor
          // do "primary" do app).
          info: '#8272ee',
          'info-content': '#12111a',
          success: '#4bd4a2',
          'success-content': '#0d1b16',
          warning: '#eeb14b',
          'warning-content': '#1e1a10',
          error: '#f27575',
          'error-content': '#1e1414',
          '--rounded-box': '16px',
          '--rounded-btn': '9px',
          '--rounded-badge': '6px',
        },
      },
      {
        'aad-light': {
          'color-scheme': 'light',
          primary: '#6a55d6',
          'primary-content': '#ffffff',
          secondary: '#6a55d6',
          'secondary-content': '#ffffff',
          accent: '#6a55d6',
          'accent-content': '#ffffff',
          neutral: '#6f7788',
          'neutral-content': '#ffffff',
          'base-100': '#ffffff', // = --bg-card
          'base-200': '#eef0f5', // = --bg-sunken
          // base-300 não tem um token dedicado no spec. Diferente do escuro (ver comentário
          // acima), este valor já nasceu na direção certa (base-100 > base-200 > base-300,
          // monotônico) e cobre os mesmos 5 usos como fundo sólido — mantido como estava:
          // --border-default (rgba(16,18,24,.10)) achatado sobre --bg-window.
          'base-300': '#dedfe3',
          'base-content': '#14161d',
          info: '#6a55d6',
          'info-content': '#ffffff',
          success: '#0e7a58',
          'success-content': '#ffffff',
          warning: '#8a5d0b',
          'warning-content': '#ffffff',
          error: '#c53b3b',
          'error-content': '#ffffff',
          '--rounded-box': '16px',
          '--rounded-btn': '9px',
          '--rounded-badge': '6px',
        },
      },
    ],
    darkTheme: 'aad-dark',
    base: true,
    styled: true,
    utils: true,
    logs: false,
  },
}
