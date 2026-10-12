import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import AddAnime from '../../src/routes/AddAnime.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AniListSearchResult, BlockReason } from '../../src/lib/api/client.js'

// AddAnime.svelte only reads these exports from the client; ApiError has to be the real class
// because the add's error handling does `instanceof`.
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

/** Types the term, submits through the button and waits for the search to resolve. */
async function search(term = 'one piece'): Promise<void> {
  const input = screen.getByRole('searchbox')
  await fireEvent.input(input, { target: { value: term } })
  await fireEvent.click(screen.getByRole('button', { name: /^search$|^buscar$/i }))
  await waitFor(() => expect(searchAniList).toHaveBeenCalled())
  await screen.findByText('One Piece')
}

beforeEach(() => {
  vi.clearAllMocks()
  getConfig.mockResolvedValue({ completed_anime_path: '/tmp/completed' } as never)
  searchAniList.mockResolvedValue([])
})

describe('AddAnime — card footer', () => {
  it('links to the detail when the anime is already in one of the lists', async () => {
    searchAniList.mockResolvedValue([result('tracked')])
    render(AddAnime)
    await search()

    const link = screen.getByRole('link', { name: /view status|ver status/i })
    expect(link).toHaveAttribute('href', '#/status/21')
    // The reason leaves the tooltip and becomes text: on mobile there's no hover.
    expect(screen.getByText(/already in one of your lists|já está em uma das suas listas/i)).toBeInTheDocument()
  })

  it('links to the detail when the anime was already downloaded', async () => {
    searchAniList.mockResolvedValue([result('downloaded')])
    render(AddAnime)
    await search()

    expect(screen.getByRole('link', { name: /view status|ver status/i })).toHaveAttribute(
      'href',
      '#/status/21',
    )
  })

  it('offers no detail for a blacklisted anime — it has no page', async () => {
    searchAniList.mockResolvedValue([result('blacklist')])
    render(AddAnime)
    await search()

    expect(screen.queryByRole('link', { name: /view status|ver status/i })).toBeNull()
    expect(screen.getByRole('button', { name: /^add$|^adicionar$/i })).toBeDisabled()
  })

  it('swaps the Add button for View status as soon as the anime goes in', async () => {
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

describe('AddAnime — AniList link', () => {
  it('opens the anime page in a new tab, without passing the referrer', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)
    await search()

    const link = screen.getByRole('link', { name: /one piece/i })
    expect(link).toHaveAttribute('href', 'https://anilist.co/anime/21')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })
})

describe('AddAnime — unreleased', () => {
  it('hides the unreleased ones by default and redoes the search when the toggle turns on', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)
    await search()

    expect(searchAniList).toHaveBeenLastCalledWith('one piece', false, expect.anything())

    await fireEvent.click(screen.getByRole('switch'))

    await waitFor(() =>
      expect(searchAniList).toHaveBeenLastCalledWith('one piece', true, expect.anything()),
    )
  })
})

// AniList's limit is 30 req/min and the search is disposable: one request per keystroke made a slow
// typist burn their own bucket and see the next search refused.
describe('AddAnime — AniList budget', () => {
  it('does not search while typing, only on submit', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)

    const input = screen.getByRole('searchbox')
    for (const term of ['one', 'one ', 'one p', 'one pi']) {
      await fireEvent.input(input, { target: { value: term } })
    }
    expect(searchAniList).not.toHaveBeenCalled()

    await fireEvent.click(screen.getByRole('button', { name: /^search$|^buscar$/i }))
    await waitFor(() => expect(searchAniList).toHaveBeenCalledTimes(1))
  })

  it('Enter in the field submits the form', async () => {
    searchAniList.mockResolvedValue([result('')])
    render(AddAnime)

    const input = screen.getByRole('searchbox')
    await fireEvent.input(input, { target: { value: 'one piece' } })
    await fireEvent.submit(input.closest('form') as HTMLFormElement)

    await waitFor(() => expect(searchAniList).toHaveBeenCalledWith('one piece', false, expect.anything()))
  })

  it('keeps the button disabled below 3 characters', async () => {
    render(AddAnime)
    const button = screen.getByRole('button', { name: /^search$|^buscar$/i })
    expect(button).toBeDisabled()

    await fireEvent.input(screen.getByRole('searchbox'), { target: { value: 'one' } })
    expect(button).toBeEnabled()
  })
})
