
/*
 * Cores semânticas (fase 0 do redesign de UI). Fonte da verdade: src/lib/design/tokens.css.
 *
 * Cada entrada abaixo usa `rgb(var(--x-rgb) / <alpha-value>)` (não um hex fixo) para as
 * cores de acento/status, o que habilita o modificador de opacidade do Tailwind
 * (`bg-ok/12`, `border-ok-tint/28`, ...). Superfícies e texto não precisam de opacidade
 * variável, então usam `var(--x)` direto.
 *
 * `heading`/`body` correspondem a --text-primary/--text-secondary do tokens.css. Os nomes
 * nasceram assim para não colidir com `primary`/`secondary` do daisyUI, que reservava essas
 * duas chaves para a paleta de marca dele. O daisyUI saiu, mas os nomes ficam: renomear hoje
 * tocaria toda tela sem mudar um pixel.
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
      // §4.3 — escala tipográfica. Registrada aqui como utilitário `text-*`. O spec dá faixas,
      // não valores únicos. `line-height` não é dado pelo spec, então é deixado de fora
      // (comportamento padrão do Tailwind) em vez de inventar um valor. `rótulo mono` também
      // pede maiúsculo — isso é `uppercase` aplicado pelo consumidor junto com
      // `text-mono-label`, não faz parte do tuple de font-size do Tailwind.
      //
      // AUMENTADA em ~1px (texto pequeno) a ~2px (números grandes) sobre os valores do spec, a
      // pedido do usuário: no tamanho anterior a UI inteira lia como pequena demais. Toda a
      // escala subiu junta, e não só as entradas reclamadas, para as proporções entre título /
      // corpo / secundário continuarem as mesmas do handoff. Os poucos tamanhos escritos à mão
      // como `text-[Npx]` nas telas subiram na mesma proporção.
      fontSize: {
        // título de tela: spec 20–22px/800, -.01em
        'screen-title': ['24px', { letterSpacing: '-0.01em', fontWeight: '800' }],
        // número herói: spec 46px/600, -.03em
        'hero-number': ['48px', { letterSpacing: '-0.03em', fontWeight: '600' }],
        // número de card: spec 22–30px/650 (650 é literal do spec; não há peso 650 carregado —
        // o navegador casa para o peso mais próximo disponível)
        'card-number': ['32px', { fontWeight: '650' }],
        // título de card: spec 14–15px/700
        'card-title': ['16px', { fontWeight: '700' }],
        // corpo: spec 13–13.5px/600
        copy: ['14.5px', { fontWeight: '600' }],
        // secundário: spec 12–12.5px/500
        caption: ['13.5px', { fontWeight: '500' }],
        // rótulo mono: spec 10.5–11px/600, letter-spacing .10–.12em, maiúsculo
        'mono-label': ['12px', { letterSpacing: '0.12em', fontWeight: '600' }],
      },
    },
  },
  plugins: [],
}
