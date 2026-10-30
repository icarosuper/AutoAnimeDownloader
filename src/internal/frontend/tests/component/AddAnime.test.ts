import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import { get } from 'svelte/store'
import AddAnime from '../../src/routes/AddAnime.svelte'
import * as client from '../../src/lib/api/client.js'
import { ApiError, type AniListSearchResult, type BlockReason, type TorrentCandidate } from '../../src/lib/api/client.js'
import { pickerCache } from '../../src/lib/stores/pickerCache.js'
import { toast } from '../../src/lib/stores/toast.js'
import * as router from 'svelte-spa-router'

// AddAnime.svelte only reads these exports from the client; ApiError has to be the real class
// because the add's error handling does `instanceof`.
vi.mock('../../src/lib/api/client.js', async () => {
  const actual = await vi.importActual<typeof client>('../../src/lib/api/client.js')
  return {
    ApiError: actual.ApiError,
    searchAniList: vi.fn(),
    addStandaloneAnime: vi.fn(),
    getTorrentCandidates: vi.fn(),
    getConfig: vi.fn(),
  }
})

vi.mock('svelte-spa-router', async () => {
  const actual = await vi.importActual<typeof router>('svelte-spa-router')
  return {
    ...actual,
    push: vi.fn().mockResolvedValue(undefined),
  }
})

const searchAniList = vi.mocked(client.searchAniList)
const addStandaloneAnime = vi.mocked(client.addStandaloneAnime)
const getTorrentCandidates = vi.mocked(client.getTorrentCandidates)
const getConfig = vi.mocked(client.getConfig)
const push = vi.mocked(router.push)

vi.spyOn(toast, 'success')
vi.spyOn(toast, 'info')
vi.spyOn(toast, 'error')

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

function candidate(over: Partial<TorrentCandidate> = {}): TorrentCandidate {
  return {
    name: '[SubsPlease] Frieren (01-16) [1080p]',
    magnet: 'magnet:?xt=urn:btih:aaa',
    size_bytes: 12_000_000_000,
    seeders: 42,
    leechers: 3,
    fansub: 'subsplease',
    resolution: '1080p',
    is_batch: true,
    range_start: 1,
    range_end: 16,
    nyaa_id: 12345,
    ...over,
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
  pickerCache.clear()
  getConfig.mockResolvedValue({ completed_anime_path: '/tmp/completed' } as never)
  searchAniList.mockResolvedValue([])
  // Empty first search is today's add: the existing "swaps the Add button" case still POSTs.
  getTorrentCandidates.mockResolvedValue({
    query: 'One Piece',
    media: { title: 'One Piece', format: 'TV' },
    items: [],
  })
  addStandaloneAnime.mockResolvedValue({ added: 2 })
  push.mockResolvedValue(undefined)
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

describe('AddAnime — first search', () => {
  it('empty first search adds without magnet and does not open the picker', async () => {
    searchAniList.mockResolvedValue([result('')])
    getTorrentCandidates.mockResolvedValue({
      query: 'One Piece',
      media: { title: 'One Piece', format: 'TV' },
      items: [],
    })
    addStandaloneAnime.mockResolvedValue({ added: 2 })
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() => expect(addStandaloneAnime).toHaveBeenCalled())
    expect(getTorrentCandidates).toHaveBeenCalledWith(21)
    expect(addStandaloneAnime).toHaveBeenCalledWith(21)
    expect(addStandaloneAnime.mock.calls[0][1]).toBeUndefined()
    expect(push).not.toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()
    expect(toast.info).not.toHaveBeenCalled()
    expect(get(pickerCache)).toBeNull()
  })

  it('empty first search with nothing to queue keeps the tracked-no-episodes toast', async () => {
    searchAniList.mockResolvedValue([result('')])
    getTorrentCandidates.mockResolvedValue({
      query: 'One Piece',
      media: { title: 'One Piece', format: 'TV' },
      items: [],
    })
    addStandaloneAnime.mockResolvedValue({ added: 0 })
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() => expect(addStandaloneAnime).toHaveBeenCalledWith(21))
    expect(push).not.toHaveBeenCalled()
    expect(toast.info).toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    await waitFor(() =>
      expect(screen.getByRole('link', { name: /view status|ver status/i })).toBeInTheDocument(),
    )
  })

  it('non-empty first search navigates to the picker and does not POST', async () => {
    const items = [candidate()]
    searchAniList.mockResolvedValue([result('')])
    getTorrentCandidates.mockResolvedValue({
      query: 'Frieren',
      media: { title: 'One Piece', format: 'TV' },
      items,
    })
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() => expect(push).toHaveBeenCalledWith('/add/21/torrents'))
    expect(getTorrentCandidates).toHaveBeenCalledWith(21)
    expect(addStandaloneAnime).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.info).not.toHaveBeenCalled()
    expect(get(pickerCache)).toEqual({
      mediaId: 21,
      query: 'Frieren',
      items,
      error: null,
      media: { title: 'One Piece', format: 'TV' },
    })
    // finally must drop the in-flight flag so a return from the picker is not stuck on Adding.
    expect(screen.getByRole('button', { name: /^add$|^adicionar$/i })).toBeEnabled()
  })

  it('Nyaa-down first search opens the picker with the error cached and does not POST', async () => {
    const err = new ApiError('nyaa down', 'NYAA_UNAVAILABLE', 502)
    searchAniList.mockResolvedValue([result('')])
    getTorrentCandidates.mockRejectedValue(err)
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() => expect(push).toHaveBeenCalledWith('/add/21/torrents'))
    expect(addStandaloneAnime).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(get(pickerCache)).toEqual({
      mediaId: 21,
      query: '',
      items: [],
      error: err,
      media: { title: 'One Piece', format: 'TV' },
    })
    expect(screen.getByRole('button', { name: /^add$|^adicionar$/i })).toBeEnabled()
  })

  it('other first-search errors toast and stay on the card', async () => {
    searchAniList.mockResolvedValue([result('')])
    getTorrentCandidates.mockRejectedValue(new ApiError('boom', 'INTERNAL_ERROR', 500))
    render(AddAnime)
    await search()

    await fireEvent.click(screen.getByRole('button', { name: /^add$|^adicionar$/i }))

    await waitFor(() => expect(toast.error).toHaveBeenCalled())
    expect(addStandaloneAnime).not.toHaveBeenCalled()
    expect(push).not.toHaveBeenCalled()
    expect(get(pickerCache)).toBeNull()
    expect(screen.getByRole('button', { name: /^add$|^adicionar$/i })).toBeEnabled()
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
