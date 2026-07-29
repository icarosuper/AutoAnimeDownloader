/**
 * Fonte única dos itens de navegação do shell (Fase 1 do redesign de UI, spec §5).
 *
 * Antes desta fase, `components/Layout.svelte` escrevia os seis links de navegação DUAS
 * VEZES — um bloco para o header desktop, outro para o menu mobile — com as classes de
 * estado ativo repetidas em cada bloco. `NavRail`, `NavTabBar` e `MoreMenu` consomem os
 * arrays abaixo em vez de declarar a lista de novo.
 *
 * `label` guarda a REFERÊNCIA da função de mensagem do paraglide (não o texto já resolvido).
 * Essas funções lêem o locale atual via `getLocale()` (não são stores), então precisam ser
 * chamadas dentro do idioma reativo `$: T = $locale && { ... }` já usado em todo o app — só
 * assim a troca de idioma dispara um novo render. Quem consumir este array deve mapear
 * `item.label()` dentro desse bloco reativo, não direto no template.
 */
import { Activity, Bell, Download, Ellipsis, ListOrdered, ScrollText, Settings } from '@lucide/svelte'
import * as m from './i18n/messages.js'

/** Todo ícone Lucide usado aqui tem essa mesma assinatura de componente. */
type IconComponent = typeof Activity

export interface NavItem {
  /** id estável — key do #each e sufixo de outros ids derivados (ex. inputs, testids) */
  id: string
  /** rota (hash) de destino, ex. '/status' */
  path: string
  /** paths adicionais que também marcam este item como ativo (ex. '/' para Status) */
  activePaths?: string[]
  icon: IconComponent
  /** função de mensagem i18n — chame dentro de um `$: T = $locale && {...}` reativo */
  label: () => string
}

/** Grupo de cima do rail / primeiras colunas da tab bar: Status, Downloads. */
export const primaryNavItems: NavItem[] = [
  { id: 'status', path: '/status', activePaths: ['/'], icon: Activity, label: m.nav_status },
  { id: 'downloads', path: '/downloads', icon: Download, label: m.nav_downloads },
]

/** Grupo de baixo do rail (depois do divisor) / terceira coluna da tab bar: Configurações. */
export const secondaryNavItems: NavItem[] = [
  { id: 'config', path: '/config', icon: Settings, label: m.nav_config },
]

/** Itens hospedados dentro do MoreMenu, atrás do gatilho "Mais". */
export const moreMenuItems: NavItem[] = [
  { id: 'notifications', path: '/notifications', icon: Bell, label: m.nav_notifications },
  { id: 'priorities', path: '/priorities', icon: ListOrdered, label: m.nav_priorities },
  { id: 'logs', path: '/logs', icon: ScrollText, label: m.nav_logs },
]

/** Ícone + rótulo do gatilho "Mais" em si — não é uma rota, abre o MoreMenu. */
export const moreTriggerIcon: IconComponent = Ellipsis
export const moreTriggerLabel: () => string = m.nav_more

/**
 * currentPath corresponde à rota deste item (exata ou um dos activePaths)? Aceita só o
 * subconjunto de campos usados (não `NavItem` inteiro) porque quem chama normalmente já
 * mapeou `label` de função para string resolvida (ver o `$: T = $locale && {...}` de
 * NavRail/NavTabBar/MoreMenu) antes de checar o item ativo.
 */
export function isNavItemActive(
  item: Pick<NavItem, 'path' | 'activePaths'>,
  currentPath: string,
): boolean {
  return currentPath === item.path || (item.activePaths ?? []).includes(currentPath)
}

/** currentPath é uma das rotas hospedadas no MoreMenu? Usado para destacar o gatilho "Mais". */
export function isMoreMenuActive(currentPath: string): boolean {
  return moreMenuItems.some((item) => isNavItemActive(item, currentPath))
}
