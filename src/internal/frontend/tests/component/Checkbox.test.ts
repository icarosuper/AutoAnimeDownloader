import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import Checkbox from '../../src/components/ui/Checkbox.svelte'

// The two props Fase 4 added for AnimeDetail's per-row selection. Both are easy to break
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
})
