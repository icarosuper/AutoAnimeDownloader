import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, fireEvent, within } from '@testing-library/svelte'
import ActionMenu from '../../src/components/ui/ActionMenu.svelte'
import ActionMenuSlotHost from './ActionMenu.slot-host.svelte'

const items = [
  { id: 'a', label: 'Action A' },
  { id: 'b', label: 'Action B', destructive: true },
]

// getBoundingClientRect is stubbed per-test where needed (the flip test); always restore it so
// later tests in this file get jsdom's normal all-zero rect back.
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect
afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

describe('ActionMenu', () => {
  it('is closed by default and opens the panel when the trigger is clicked', async () => {
    const { getByRole, queryByRole } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    expect(queryByRole('menu')).not.toBeInTheDocument()

    await fireEvent.click(getByRole('button', { name: 'More actions' }))

    expect(getByRole('menu')).toBeInTheDocument()
    expect(getByRole('menuitem', { name: 'Action A' })).toBeInTheDocument()
    expect(getByRole('menuitem', { name: 'Action B' })).toBeInTheDocument()
  })

  it('closes when Escape is pressed', async () => {
    const { getByRole, queryByRole } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    await fireEvent.click(getByRole('button', { name: 'More actions' }))
    expect(queryByRole('menu')).toBeInTheDocument()

    await fireEvent.keyDown(window, { key: 'Escape' })

    expect(queryByRole('menu')).not.toBeInTheDocument()
  })

  it('closes on a click outside the menu', async () => {
    const { getByRole, queryByRole } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    await fireEvent.click(getByRole('button', { name: 'More actions' }))
    expect(queryByRole('menu')).toBeInTheDocument()

    await fireEvent.click(document.body)

    expect(queryByRole('menu')).not.toBeInTheDocument()
  })

  it('closes and dispatches select with the item id when an item is chosen', async () => {
    const onSelect = vi.fn()
    const { getByRole, queryByRole } = render(ActionMenu, {
      props: { items, triggerLabel: 'More actions' },
      events: { select: onSelect },
    })
    await fireEvent.click(getByRole('button', { name: 'More actions' }))

    await fireEvent.click(getByRole('menuitem', { name: 'Action A' }))

    expect(onSelect).toHaveBeenCalledTimes(1)
    expect((onSelect.mock.calls[0][0] as CustomEvent<string>).detail).toBe('a')
    expect(queryByRole('menu')).not.toBeInTheDocument()
  })

  it('only one menu is open at a time: opening a second instance closes the first', async () => {
    // Scoped to each instance's own `container` (not the bound queries, which default to the
    // whole document.body — with two instances mounted, `first.queryByRole` would also match
    // second's panel and make this assertion meaningless).
    const first = render(ActionMenu, { props: { items, triggerLabel: 'Menu 1' } })
    const second = render(ActionMenu, { props: { items, triggerLabel: 'Menu 2' } })

    await fireEvent.click(within(first.container).getByRole('button', { name: 'Menu 1' }))
    expect(within(first.container).queryByRole('menu')).toBeInTheDocument()

    await fireEvent.click(within(second.container).getByRole('button', { name: 'Menu 2' }))

    expect(within(second.container).queryByRole('menu')).toBeInTheDocument()
    expect(within(first.container).queryByRole('menu')).not.toBeInTheDocument()
  })

  it('flips upward (bottom-full) when the panel would overflow the bottom of the viewport', async () => {
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 600 })
    HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
      const rect = { bottom: 0, top: 0, left: 0, right: 0, height: 0, width: 0, x: 0, y: 0, toJSON() {} }
      if (this.getAttribute('role') === 'menu') return { ...rect, bottom: 700, top: 500 } as DOMRect
      return rect as DOMRect
    }

    const { getByRole } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    await fireEvent.click(getByRole('button', { name: 'More actions' }))

    expect(getByRole('menu').className).toContain('bottom-full')
  })

  it('puts slotted trigger content inside the trigger button and still opens the menu', async () => {
    const { getByRole, queryByRole } = render(ActionMenuSlotHost, {
      props: { items, triggerLabel: 'Filter by type', triggerClass: 'rounded-pill px-3' },
    })

    const trigger = getByRole('button', { name: 'Airing 12' })
    expect(trigger.textContent).toContain('Airing 12')
    expect(trigger.className).toContain('rounded-pill')
    expect(queryByRole('menu')).not.toBeInTheDocument()

    await fireEvent.click(trigger)

    expect(getByRole('menu')).toBeInTheDocument()
  })

  it('keeps the kebab trigger when no slot is provided', async () => {
    const { getByRole, container } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    const trigger = getByRole('button', { name: 'More actions' })
    expect(trigger.querySelector('svg')).toBeTruthy()
    expect(container.textContent).not.toContain('Airing 12')
  })

  it('does not flip when the panel fits within the viewport', async () => {
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 900 })
    HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
      const rect = { bottom: 0, top: 0, left: 0, right: 0, height: 0, width: 0, x: 0, y: 0, toJSON() {} }
      if (this.getAttribute('role') === 'menu') return { ...rect, bottom: 400, top: 300 } as DOMRect
      return rect as DOMRect
    }

    const { getByRole } = render(ActionMenu, { props: { items, triggerLabel: 'More actions' } })
    await fireEvent.click(getByRole('button', { name: 'More actions' }))

    expect(getByRole('menu').className).toContain('top-full')
  })
})
