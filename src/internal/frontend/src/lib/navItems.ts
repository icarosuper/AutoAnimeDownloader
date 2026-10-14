/**
 * Single source of the shell's navigation items (UI redesign Phase 1, spec §5).
 *
 * Before that phase, `components/Layout.svelte` wrote the six navigation links TWICE — one block
 * for the desktop header, another for the mobile menu — with the active-state classes repeated in
 * each block. `NavRail`, `NavTabBar` and `MoreMenu` consume the arrays below instead of declaring
 * the list again.
 *
 * `label` holds the REFERENCE to the paraglide message function (not the resolved text). Those
 * functions read the current locale via `getLocale()` (they aren't stores), so they have to be
 * called inside the reactive `$: T = $locale && { ... }` block already used across the app — only
 * then does a language switch trigger a new render. Whoever consumes this array should map
 * `item.label()` inside that reactive block, not straight in the template.
 */
import { Activity, Bell, Bug, Download, Ellipsis, ListOrdered, Plus, ScrollText, Settings } from '@lucide/svelte'
import * as m from './i18n/messages.js'

/** Every Lucide icon used here has this same component signature. */
type IconComponent = typeof Activity

export interface NavItem {
  /** stable id — the #each key and the suffix of other derived ids (e.g. inputs, testids) */
  id: string
  /** target (hash) route, e.g. '/status' */
  path: string
  /** extra paths that also mark this item as active (e.g. '/' for Status) */
  activePaths?: string[]
  icon: IconComponent
  /** i18n message function — call it inside a reactive `$: T = $locale && {...}` */
  label: () => string
}

/**
 * Top group of the rail / first columns of the tab bar: Status, Add anime, Downloads.
 *
 * "Add anime" comes SECOND and with the same weight as Status — it's the entry point of any anime
 * into the library, and burying it in the MoreMenu (where it was born) hid the only action an
 * install without an AniList account has to take.
 */
export const primaryNavItems: NavItem[] = [
  { id: 'status', path: '/status', activePaths: ['/'], icon: Activity, label: m.nav_status },
  { id: 'add', path: '/add', icon: Plus, label: m.nav_add_anime },
  { id: 'downloads', path: '/downloads', icon: Download, label: m.nav_downloads },
]

/** Bottom group of the rail (after the divider) / third column of the tab bar: Configuration. */
export const secondaryNavItems: NavItem[] = [
  { id: 'config', path: '/config', icon: Settings, label: m.nav_config },
]

/** Items hosted inside the MoreMenu, behind the "More" trigger. */
export const moreMenuItems: NavItem[] = [
  { id: 'notifications', path: '/notifications', icon: Bell, label: m.nav_notifications },
  { id: 'priorities', path: '/priorities', icon: ListOrdered, label: m.nav_priorities },
  { id: 'logs', path: '/logs', icon: ScrollText, label: m.nav_logs },
  { id: 'report', path: '/report', icon: Bug, label: m.nav_report },
]

/** Icon + label of the "More" trigger itself — not a route, it opens the MoreMenu. */
export const moreTriggerIcon: IconComponent = Ellipsis
export const moreTriggerLabel: () => string = m.nav_more

/**
 * Does currentPath match this item's route (exactly or one of the activePaths)? Takes only the
 * subset of fields used (not the whole `NavItem`) because callers usually already mapped `label`
 * from a function to a resolved string (see the `$: T = $locale && {...}` of
 * NavRail/NavTabBar/MoreMenu) before checking the active item.
 */
export function isNavItemActive(
  item: Pick<NavItem, 'path' | 'activePaths'>,
  currentPath: string,
): boolean {
  return currentPath === item.path || (item.activePaths ?? []).includes(currentPath)
}

/** Is currentPath one of the routes hosted in the MoreMenu? Used to highlight the "More" trigger. */
export function isMoreMenuActive(currentPath: string): boolean {
  return moreMenuItems.some((item) => isNavItemActive(item, currentPath))
}
