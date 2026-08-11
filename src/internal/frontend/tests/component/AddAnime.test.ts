import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import AddAnime from '../../src/routes/AddAnime.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AniListSearchResult, BlockReason } from '../../src/lib/api/client.js'

// AddAnime.svelte só lê estes exports do client; ApiError precisa ser a classe real porque o
// tratamento de erro do add faz `instanceof`.
vi.mock('../../src/lib/api/client.js', async () => {
  const actual = await vi.importActual<typeof client>('../../src/lib/api/client.js')
  return {
    ApiError: actual.ApiError,
    searchAniList: vi.fn(),
    addStandaloneAnime: vi.fn(),
    getConfig: vi.fn(),
  }
})

const searchAniList = vi.mocked(client.searchAniList)
const addStandaloneAnime = vi.mocked(client.addStandaloneAnime)
const getConfig = vi.mocked(client.getConfig)

function result(blockReason: BlockReason, id = 21): AniListSearchResult {
  return {
    id,
    title: 'One Piece',
    format: 'TV',
    status: 'RELEASING',
    year: 1999,
    episodes: 0,
    cover: '',
    block_reason: blockReason,
  }
}

/** Digita o termo e espera a busca (debounce de 300ms) resolver. */
async function search(term = 'one piece'): Promise<void> {
  const input = screen.getByRole('searchbox')
  await fireEvent.input(input, { target: { value: term } })
  await waitFor(() => expect(searchAniList).toHaveBeenCalled())
  await screen.findByText('One Piece')
}

beforeEach(() => {
  vi.clearAllMocks()
  getConfig.mockResolvedValue({ completed_anime_path: '/tmp/completed' } as never)
  searchAniList.mockResolvedValue([])
})

describe('AddAnime — rodapé do card', () => {
  it('leva ao detalhe quando o anime já está na lista do AniList', async () => {
    searchAniList.mockResolvedValue([result('tracked')])
    render(AddAnime)
    await search()

    const link = screen.getByRole('link', { name: /view status|ver status/i })
    expect(link).toHaveAttribute('href', '#/status/21')
    // O motivo sai do tooltip e vira texto: no mobile não existe hover.
    expect(screen.getByText(/already in your anilist list|já está na sua lista/i)).toBeInTheDocument()
  })

  it('leva ao detalhe quando o anime já foi baixado', async () => {
    searchAniList.mockResolvedValue([result('downloaded')])
    render(AddAnime)
    await search()

    expect(screen.getByRole('link', { name: /view status|ver status/i })).toHaveAttribute(
      'href',
      '#/status/21',
    )
  })

  it('não oferece detalhe para um anime na blacklist — ele não tem página', async () => {
    searchAniList.mockResolvedValue([result('blacklist')])
    render(AddAnime)
    await search()

    expect(screen.queryByRole('link', { name: /view status|ver status/i })).toBeNull()
    expect(screen.getByRole('button', { name: /^add$|^adicionar$/i })).toBeDisabled()
  })

  it('troca o botão Adicionar por Ver status assim que o anime entra', async () => {
    searchAniList.mockResolvedValue([result('')])
    addStandaloneAnime.mockResolvedValue({ added: 2 })
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() =>
      expect(screen.getByRole('link', { name: /view status|ver status/i })).toHaveAttribute(
        'href',
        '#/status/21',
      ),
    )
  })
})

describe('AddAnime — link do AniList', () => {
  it('abre a página do anime em nova aba, sem passar o referrer', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)
    await search()

    const link = screen.getByRole('link', { name: /one piece/i })
    expect(link).toHaveAttribute('href', 'https://anilist.co/anime/21')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })
})

describe('AddAnime — não lançados', () => {
  it('esconde os não lançados por padrão e refaz a busca quando o toggle liga', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)
    await search()

    expect(searchAniList).toHaveBeenLastCalledWith('one piece', false, expect.anything())

    await fireEvent.click(screen.getByRole('switch'))

    // Sem debounce: o toggle é clique, não digitação.
    await waitFor(() =>
      expect(searchAniList).toHaveBeenLastCalledWith('one piece', true, expect.anything()),
    )
  })
})
