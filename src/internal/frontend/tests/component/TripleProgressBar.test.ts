import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import TripleProgressBar from '../../src/components/ui/TripleProgressBar.svelte'

// As props sao CUMULATIVAS; a legenda mostra os DELTAS. O que estes testes protegem e o
// casamento palavra <-> cor: cada termo tem que carregar a classe do segmento que ele mede,
// senao a legenda colorida mente (era o bug que ela veio consertar — decisions.md #88).
const props = { watched: 5, downloaded: 10, released: 12, total: 13 }

describe('TripleProgressBar', () => {
  it('quebra os cumulativos em deltas na legenda', () => {
    const { getByText } = render(TripleProgressBar, { props })
    expect(getByText('5 watched')).toBeInTheDocument()
    expect(getByText('5 to watch')).toBeInTheDocument() // 10 - 5
    expect(getByText('2 to download')).toBeInTheDocument() // 12 - 10
    expect(getByText('1 unreleased')).toBeInTheDocument() // 13 - 12
  })

  it('cada termo usa a cor do seu segmento', () => {
    const { getByText } = render(TripleProgressBar, { props })
    expect(getByText('5 watched')).toHaveClass('text-ok')
    expect(getByText('5 to watch')).toHaveClass('text-accent')
    expect(getByText('2 to download')).toHaveClass('text-warn')
  })

  it('largura do segmento e o mesmo numero que a legenda mostra', () => {
    const { container } = render(TripleProgressBar, { props })
    const widths = [...container.querySelectorAll<HTMLElement>('.bg-ok, .bg-accent, .bg-warn')].map(
      (el) => el.style.width,
    )
    const pct = (n: number) => `${(n / 13) * 100}%`
    expect(widths).toEqual([pct(5), pct(5), pct(2)])
  })

  it('nada assistido e nada faltando nao gera termo negativo', () => {
    const { getByText } = render(TripleProgressBar, {
      props: { watched: 0, downloaded: 8, released: 8, total: 8 },
    })
    expect(getByText('0 watched')).toBeInTheDocument()
    expect(getByText('8 to watch')).toBeInTheDocument()
    expect(getByText('0 to download')).toBeInTheDocument()
    expect(getByText('0 unreleased')).toBeInTheDocument()
  })
})
