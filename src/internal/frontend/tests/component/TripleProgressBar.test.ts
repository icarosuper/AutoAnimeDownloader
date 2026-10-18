import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import TripleProgressBar from '../../src/components/ui/TripleProgressBar.svelte'

// The props are CUMULATIVE; the legend shows the DELTAS. What these tests protect is the
// word <-> color match: each term has to carry the class of the segment it measures,
// otherwise the colored legend lies (that was the bug it came to fix — decisions.md #88).
const props = { watched: 5, downloaded: 10, released: 12, total: 13 }

describe('TripleProgressBar', () => {
  it('breaks the cumulatives into deltas in the legend', () => {
    const { getByText } = render(TripleProgressBar, { props })
    expect(getByText('5 watched')).toBeInTheDocument()
    expect(getByText('5 to watch')).toBeInTheDocument() // 10 - 5
    expect(getByText('2 to download')).toBeInTheDocument() // 12 - 10
    expect(getByText('1 unreleased')).toBeInTheDocument() // 13 - 12
  })

  it('each term uses the color of its segment', () => {
    const { getByText } = render(TripleProgressBar, { props })
    expect(getByText('5 watched')).toHaveClass('text-ok')
    expect(getByText('5 to watch')).toHaveClass('text-accent')
    expect(getByText('2 to download')).toHaveClass('text-warn')
  })

  it('segment width is the same number the legend shows', () => {
    const { container } = render(TripleProgressBar, { props })
    const widths = [...container.querySelectorAll<HTMLElement>('.bg-ok, .bg-accent, .bg-warn')].map(
      (el) => el.style.width,
    )
    const pct = (n: number) => `${(n / 13) * 100}%`
    expect(widths).toEqual([pct(5), pct(5), pct(2)])
  })

  it('nothing watched and nothing missing does not produce a negative term', () => {
    const { getByText } = render(TripleProgressBar, {
      props: { watched: 0, downloaded: 8, released: 8, total: 8, keepZeros: true },
    })
    expect(getByText('0 watched')).toBeInTheDocument()
    expect(getByText('8 to watch')).toBeInTheDocument()
    expect(getByText('0 to download')).toBeInTheDocument()
    expect(getByText('0 unreleased')).toBeInTheDocument()
  })

  it('by default omits a zero term (anime row)', () => {
    const { getByText, queryByText } = render(TripleProgressBar, {
      props: { watched: 0, downloaded: 8, released: 8, total: 8 },
    })
    expect(getByText('8 to watch')).toBeInTheDocument()
    expect(queryByText('0 watched')).not.toBeInTheDocument()
    expect(queryByText('0 to download')).not.toBeInTheDocument()
    expect(queryByText('0 unreleased')).not.toBeInTheDocument()
  })

  it('keepZeros keeps all four terms (Library card)', () => {
    const { container } = render(TripleProgressBar, { props: { ...props, keepZeros: true } })
    expect(container.querySelector('p')?.textContent).toBe(
      '5 watched · 5 to watch · 2 to download · 1 unreleased',
    )
  })

  it('separator does not linger when a term vanishes', () => {
    const { container } = render(TripleProgressBar, {
      props: { watched: 5, downloaded: 5, released: 10, total: 13 },
    })
    expect(container.querySelector('p')?.textContent).toBe(
      '5 watched · 5 to download · 3 unreleased',
    )
  })

  // An anime with no released episode zeros everything; an empty <p> would shrink the row
  // and make the progress-column height oscillate, which is what LIST_GRID cannot have.
  it('all zeros still renders a legend line', () => {
    const { container } = render(TripleProgressBar, {
      props: { watched: 0, downloaded: 0, released: 0, total: 0 },
    })
    expect(container.querySelector('p')?.textContent).toBe('0 watched')
  })
})
