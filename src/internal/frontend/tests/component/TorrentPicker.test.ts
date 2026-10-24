import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte'
import TorrentPicker from '../../src/routes/TorrentPicker.svelte'
import * as client from '../../src/lib/api/client.js'
import { ApiError, type TorrentCandidate } from '../../src/lib/api/client.js'
import { pickerCache } from '../../src/lib/stores/pickerCache.js'
import { locale } from '../../src/lib/stores/locale.js'
import * as router from 'svelte-spa-router'

vi.mock('../../src/lib/api/client.js', async () => {
  const actual = await vi.importActual<typeof client>('../../src/lib/api/client.js')
  return {
    ApiError: actual.ApiError,
    getTorrentCandidates: vi.fn(),
    getTorrentCandidateDetail: vi.fn(),
    addStandaloneAnime: vi.fn(),
    replaceAnimeWithMagnet: vi.fn(),
  }
})

vi.mock('svelte-spa-router', async () => {
  const actual = await vi.importActual<typeof router>('svelte-spa-router')
  return {
    ...actual,
    pop: vi.fn().mockResolvedValue(undefined),
    push: vi.fn().mockResolvedValue(undefined),
  }
})

const getTorrentCandidates = vi.mocked(client.getTorrentCandidates)
const getTorrentCandidateDetail = vi.mocked(client.getTorrentCandidateDetail)
const addStandaloneAnime = vi.mocked(client.addStandaloneAnime)
const replaceAnimeWithMagnet = vi.mocked(client.replaceAnimeWithMagnet)
const pop = vi.mocked(router.pop)
const push = vi.mocked(router.push)

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

function detail(over: Partial<{ description_html: string; files: { name: string; size_bytes: number }[]; nyaa_url: string }> = {}) {
  return {
    description_html: '<p>A very good pack</p>',
    files: [{ name: 'Frieren_01.mkv', size_bytes: 1_000_000 }],
    nyaa_url: 'https://nyaa.si/view/12345',
    ...over,
  }
}

async function mountPicker(hash = '#/add/21/torrents') {
  window.location.hash = hash
  render(TorrentPicker)
  await waitFor(() => expect(getTorrentCandidates).toHaveBeenCalled())
  await screen.findAllByRole('option')
}

beforeEach(() => {
  vi.clearAllMocks()
  locale.set('en')
  pickerCache.clear()
  window.location.hash = '#/add/21/torrents'
  getTorrentCandidates.mockResolvedValue({
    query: 'Frieren',
    items: [
      candidate(),
      candidate({ name: 'Other pack', nyaa_id: 222, magnet: 'magnet:?xt=urn:btih:bbb' }),
    ],
  })
  getTorrentCandidateDetail.mockResolvedValue(detail())
  addStandaloneAnime.mockResolvedValue({ added: 2 })
  replaceAnimeWithMagnet.mockResolvedValue(undefined)
  pop.mockResolvedValue(undefined)
  push.mockResolvedValue(undefined)
})

describe('TorrentPicker — list', () => {
  it('selects the first row as recommended and fetches its Nyaa page', async () => {
    await mountPicker()

    const options = screen.getAllByRole('option')
    expect(options[0]).toHaveAttribute('aria-selected', 'true')
    expect(within(options[0]).getByText('Recommended')).toBeInTheDocument()
    expect(within(options[1]).queryByText('Recommended')).toBeNull()
    expect(options[1]).toHaveAttribute('aria-selected', 'false')

    await waitFor(() =>
      expect(getTorrentCandidateDetail).toHaveBeenCalledWith(21, 12345, expect.anything()),
    )
    expect(getTorrentCandidateDetail).toHaveBeenCalledTimes(1)
  })

  it('Enter on the query field refetches with q= and recommends the new index 0', async () => {
    getTorrentCandidates
      .mockResolvedValueOnce({
        query: 'Frieren',
        items: [candidate({ name: 'Old pack', nyaa_id: 1 })],
      })
      .mockResolvedValueOnce({
        query: 'frieren batch',
        items: [candidate({ name: 'New pack', nyaa_id: 99 })],
      })

    await mountPicker()
    await screen.findByText('Old pack')

    const field = screen.getByRole('searchbox')
    await fireEvent.input(field, { target: { value: 'frieren batch' } })
    await fireEvent.submit(field.closest('form') as HTMLFormElement)

    await waitFor(() =>
      expect(getTorrentCandidates).toHaveBeenLastCalledWith(21, 'frieren batch', expect.anything()),
    )
    const options = screen.getAllByRole('option')
    expect(within(options[0]).getByText('New pack')).toBeInTheDocument()
    expect(within(options[0]).getByText('Recommended')).toBeInTheDocument()
    await waitFor(() =>
      expect(getTorrentCandidateDetail).toHaveBeenLastCalledWith(21, 99, expect.anything()),
    )
  })

  it('Enter on an empty query field does not request', async () => {
    await mountPicker()
    expect(getTorrentCandidates).toHaveBeenCalledTimes(1)

    const field = screen.getByRole('searchbox')
    await fireEvent.input(field, { target: { value: '' } })
    await fireEvent.submit(field.closest('form') as HTMLFormElement)

    expect(getTorrentCandidates).toHaveBeenCalledTimes(1)
  })
})

describe('TorrentPicker — confirm', () => {
  it('POSTs the selected magnet with name, nyaa_id and size_bytes', async () => {
    await mountPicker()
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')

    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))

    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, {
        magnet: 'magnet:?xt=urn:btih:aaa',
        name: '[SubsPlease] Frieren (01-16) [1080p]',
        nyaa_id: 12345,
        size_bytes: 12_000_000_000,
      }),
    )
    expect(pop).toHaveBeenCalled()
  })

  it('POSTs the pasted magnet instead of the selected row when that field is non-empty', async () => {
    await mountPicker()
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')

    await fireEvent.input(screen.getByPlaceholderText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:pasted' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))

    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, { magnet: 'magnet:?xt=urn:btih:pasted' }),
    )
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('name')
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('nyaa_id')
    expect(addStandaloneAnime.mock.calls[0][1]).not.toHaveProperty('size_bytes')
  })

  it('replace mode confirms through replaceAnimeWithMagnet and returns to the detail', async () => {
    await mountPicker('#/status/21/torrents')
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')

    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))

    await waitFor(() =>
      expect(replaceAnimeWithMagnet).toHaveBeenCalledWith(21, 'magnet:?xt=urn:btih:aaa'),
    )
    expect(addStandaloneAnime).not.toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith('/status/21')
  })
})

describe('TorrentPicker — cache', () => {
  it('uses a matching in-memory cache and does not refetch candidates', async () => {
    pickerCache.set({
      mediaId: 21,
      query: 'cached',
      items: [candidate({ name: 'Cached pack', nyaa_id: 777 })],
      error: null,
    })
    window.location.hash = '#/add/21/torrents'
    render(TorrentPicker)

    expect(await screen.findByText('Cached pack')).toBeInTheDocument()
    expect(getTorrentCandidates).not.toHaveBeenCalled()
    expect(screen.getByRole('searchbox')).toHaveValue('cached')
    await waitFor(() =>
      expect(getTorrentCandidateDetail).toHaveBeenCalledWith(21, 777, expect.anything()),
    )
  })
})

describe('TorrentPicker — description pane', () => {
  it('keeps the list and Confirm when the Nyaa page fails', async () => {
    getTorrentCandidateDetail.mockRejectedValue(new ApiError('nyaa down', 'NYAA_UNAVAILABLE', 502))
    await mountPicker()
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')
    expect(await screen.findByText('Could not load the Nyaa page.')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open on Nyaa' })).toHaveAttribute(
      'href',
      'https://nyaa.si/view/12345',
    )
    expect(screen.getByRole('button', { name: 'Download this' })).toBeEnabled()
  })
})
