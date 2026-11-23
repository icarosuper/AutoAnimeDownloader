import { describe, it, expect } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import Checkbox from '../../src/components/ui/Checkbox.svelte'

// The two props Phase 4 added for AnimeDetail's per-row selection. Both are easy to break
// silently: `labelHidden` must keep the accessible name (not drop the label), and
// `indeterminate` is a DOM *property* with no HTML attribute, so it only ever works if it
// reaches the real <input> underneath the custom box.
describe('Checkbox', () => {
  it('keeps a hidden label in the accessibility tree', () => {
    render(Checkbox, { props: { label: 'Select episode 3', labelHidden: true } })

    const box = screen.getByRole('checkbox', { name: 'Select episode 3' })
    expect(box).toBeInTheDocument()
    // Visually hidden, but still named — that is the whole point of the prop.
    expect(screen.getByText('Select episode 3')).toHaveClass('sr-only')
  })

  it('shows the label as visible text by default', () => {
    render(Checkbox, { props: { label: 'Delete files' } })
    expect(screen.getByText('Delete files')).not.toHaveClass('sr-only')
  })

  it('sets the indeterminate DOM property on the underlying input', () => {
    render(Checkbox, { props: { label: 'Select all', indeterminate: true } })

    const box = screen.getByRole('checkbox', { name: 'Select all' }) as HTMLInputElement
    expect(box.indeterminate).toBe(true)
    expect(box.checked).toBe(false)
  })

  it('is not indeterminate by default', () => {
    render(Checkbox, { props: { label: 'Select all' } })
    const box = screen.getByRole('checkbox', { name: 'Select all' }) as HTMLInputElement
    expect(box.indeterminate).toBe(false)
  })

  // Controlled mode: the parent owns the state, so a click whose handler leaves it unchanged
  // must not leave the box showing the other value.
  it('reverts to the parent value when onchange leaves it unchanged', async () => {
    let calls = 0
    render(Checkbox, { props: { label: 'Select all', checked: false, onchange: () => { calls++ } } })
    const box = screen.getByRole('checkbox', { name: 'Select all' }) as HTMLInputElement

    await fireEvent.click(box)
    await tick()

    expect(calls).toBe(1)
    expect(box.checked).toBe(false)
  })

  it('still restores the indeterminate dash the click cleared, in controlled mode', async () => {
    render(Checkbox, { props: { label: 'Select all', indeterminate: true, onchange: () => {} } })
    const box = screen.getByRole('checkbox', { name: 'Select all' }) as HTMLInputElement

    await fireEvent.click(box)
    await tick()

    expect(box.indeterminate).toBe(true)
  })

  it('toggles itself without onchange (the bind:checked mode)', async () => {
    render(Checkbox, { props: { label: 'Delete files' } })
    const box = screen.getByRole('checkbox', { name: 'Delete files' }) as HTMLInputElement

    await fireEvent.click(box)
    expect(box.checked).toBe(true)
  })
})
