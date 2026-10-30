import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, within } from '@testing-library/svelte'
import TorrentPicker from '../../src/routes/TorrentPicker.svelte'
import * as client from '../../src/lib/api/client.js'
import { ApiError, type TorrentCandidate, type TorrentCandidateMedia } from '../../src/lib/api/client.js'
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

function media(over: Partial<TorrentCandidateMedia> = {}): TorrentCandidateMedia {
  return {
    title: "Frieren: Beyond Journey's End",
    cover_image: 'https://example/cover.jpg',
    format: 'TV',
    episodes: 28,
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
    media: media(),
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
        media: media(),
        items: [candidate({ name: 'Old pack', nyaa_id: 1 })],
      })
      .mockResolvedValueOnce({
        query: 'frieren batch',
        media: media(),
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

  it('truncates a long torrent name inside a min-w-0 list card', async () => {
    const long =
      '[EMBER] Kaguya-sama: Love is War (2019-2020) (Season 1+2) [BDRip] [1080p Dual Audio HEVC 10 bits] (Kaguya-sama wa Kokurasetai: Tensai-tachi no Renai Zunousen)'
    getTorrentCandidates.mockResolvedValue({
      query: 'Kaguya',
      media: media(),
      items: [candidate({ name: long })],
    })
    await mountPicker()

    const title = screen.getByTitle(long)
    expect(title.className).toMatch(/\btruncate\b/)
    expect(title.className).toMatch(/\bflex-1\b/)
    expect(title.className).toMatch(/\bmin-w-0\b/)
    const listbox = screen.getByRole('listbox')
    expect(listbox.className).toMatch(/\bmin-w-0\b/)
    expect(listbox.closest('.rounded-card')?.className).toMatch(/\bmin-w-0\b/)
    expect(screen.getByRole('option').className).toMatch(/\bmin-w-0\b/)
  })

  it('moves a visible selected highlight to the clicked row', async () => {
    await mountPicker()
    const options = screen.getAllByRole('option')
    expect(options[0].className).toMatch(/\bring-accent\b/)
    expect(options[1].className).not.toMatch(/\bring-accent\b/)

    await fireEvent.click(options[1])
    expect(options[1]).toHaveAttribute('aria-selected', 'true')
    expect(options[1].className).toMatch(/\bring-accent\b/)
    expect(options[0]).toHaveAttribute('aria-selected', 'false')
    expect(options[0].className).not.toMatch(/\bring-accent\b/)
  })

  it('rounds the selected torrent highlight', async () => {
    await mountPicker()
    const options = screen.getAllByRole('option')
    expect(options[0].className).toMatch(/\brounded-field\b/)
    expect(options[1].className).toMatch(/\brounded-field\b/)
  })

  it('caps the torrent listbox so long results scroll', async () => {
    await mountPicker()
    const listbox = screen.getByRole('listbox')
    expect(listbox.className).toMatch(/\bmax-h-80\b/)
    expect(listbox.className).toMatch(/\boverflow-y-auto\b/)
    expect(listbox.className).toMatch(/\bp-1\.5\b/)
  })
})

describe('TorrentPicker — header', () => {
  it('shows media.title and back is ← Add anime on add', async () => {
    await mountPicker()
    expect(screen.getByText("Frieren: Beyond Journey's End")).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '← Add anime' })).toHaveAttribute('href', '#/add')
  })

  it('replace back link is ← {title}', async () => {
    await mountPicker('#/status/21/torrents')
    expect(screen.getByRole('link', { name: "← Frieren: Beyond Journey's End" })).toHaveAttribute(
      'href',
      '#/status/21',
    )
  })
})

describe('TorrentPicker — search', () => {
  it('has a Nyaa search label and a Search button; Enter still refetches; empty submit does not', async () => {
    await mountPicker()
    expect(screen.getByText('Nyaa search')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument()

    const field = screen.getByRole('searchbox')
    await fireEvent.input(field, { target: { value: '' } })
    await fireEvent.submit(field.closest('form') as HTMLFormElement)
    expect(getTorrentCandidates).toHaveBeenCalledTimes(1)

    await fireEvent.input(field, { target: { value: 'frieren batch' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() =>
      expect(getTorrentCandidates).toHaveBeenLastCalledWith(21, 'frieren batch', expect.anything()),
    )
  })
})

describe('TorrentPicker — pack count', () => {
  it('shows the pack count for a 2-row list', async () => {
    await mountPicker()
    expect(screen.getByText('2 packs · single episodes stay off this screen')).toBeInTheDocument()
  })

  it('shows the pack count for a 1-row list', async () => {
    getTorrentCandidates.mockResolvedValue({
      query: 'Frieren',
      media: media(),
      items: [candidate()],
    })
    await mountPicker()
    expect(screen.getByText('1 packs · single episodes stay off this screen')).toBeInTheDocument()
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

  it('Download this POSTs the selected row even when the magnet field is filled', async () => {
    await mountPicker()
    await screen.findByText('[SubsPlease] Frieren (01-16) [1080p]')
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:pasted' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))
    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, {
        magnet: 'magnet:?xt=urn:btih:aaa',
        name: '[SubsPlease] Frieren (01-16) [1080p]',
        nyaa_id: 12345,
        size_bytes: 12_000_000_000,
      }),
    )
    expect(addStandaloneAnime.mock.calls[0][1]).toHaveProperty('name')
  })

  it('Download magnet is disabled when the field is empty', async () => {
    await mountPicker()
    expect(screen.getByRole('button', { name: 'Download magnet' })).toBeDisabled()
  })

  it('Download magnet stays disabled for a non-magnet string', async () => {
    await mountPicker()
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'https://nyaa.si/view/1' },
    })
    expect(screen.getByRole('button', { name: 'Download magnet' })).toBeDisabled()
  })

  it('Download magnet POSTs { magnet } only', async () => {
    await mountPicker()
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download magnet' }))
    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, {
        magnet: 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567',
      }),
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

  it('replace: list button uses the selected magnet; magnet button uses the pasted URI', async () => {
    await mountPicker('#/status/21/torrents')
    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))
    await waitFor(() =>
      expect(replaceAnimeWithMagnet).toHaveBeenCalledWith(21, 'magnet:?xt=urn:btih:aaa'),
    )

    replaceAnimeWithMagnet.mockClear()
    await fireEvent.input(screen.getByLabelText('Or paste a magnet URI'), {
      target: { value: 'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Download magnet' }))
    await waitFor(() =>
      expect(replaceAnimeWithMagnet).toHaveBeenCalledWith(
        21,
        'magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567',
      ),
    )
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

  it('does not throw when the Nyaa page sends files: null, and Confirm still works', async () => {
    getTorrentCandidateDetail.mockResolvedValue({
      description_html: '<p>No file tree</p>',
      files: null,
      nyaa_url: 'https://nyaa.si/view/12345',
    } as unknown as Awaited<ReturnType<typeof client.getTorrentCandidateDetail>>)

    await mountPicker()
    expect(await screen.findByText('No file tree')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Description' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Files' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Download this' })).toBeEnabled()

    await fireEvent.click(screen.getByRole('button', { name: 'Download this' }))
    await waitFor(() =>
      expect(addStandaloneAnime).toHaveBeenCalledWith(21, {
        magnet: 'magnet:?xt=urn:btih:aaa',
        name: '[SubsPlease] Frieren (01-16) [1080p]',
        nyaa_id: 12345,
        size_bytes: 12_000_000_000,
      }),
    )
  })

  it('shows a Description heading above the Nyaa html', async () => {
    await mountPicker()
    const heading = await screen.findByRole('heading', { name: 'Description' })
    expect(heading.nextElementSibling?.innerHTML).toContain('A very good pack')
  })

  it('removes a description image that fails to load', async () => {
    getTorrentCandidateDetail.mockResolvedValue(
      detail({
        description_html:
          '<p>Discord: <a href="https://discord.gg/x"><img src="https://example.com/badge.png" alt="alt text"></a></p><p><img src="https://example.com/shot.png" alt="alt text"></p>',
      }),
    )
    await mountPicker()
    const heading = await screen.findByRole('heading', { name: 'Description' })
    const pane = heading.nextElementSibling as HTMLElement
    const imgs = within(pane).getAllByRole('img')
    expect(imgs).toHaveLength(2)
    for (const img of imgs) {
      fireEvent.error(img)
    }
    expect(within(pane).queryByRole('img')).toBeNull()
    expect(within(pane).queryByText('alt text')).toBeNull()
    expect(pane.querySelector('a[href="https://discord.gg/x"]')).toBeNull()
  })

  it('pads the description pane so the scrollbar sits off the text', async () => {
    await mountPicker()
    const heading = await screen.findByRole('heading', { name: 'Description' })
    expect(heading.nextElementSibling?.className).toMatch(/\bpr-3\b/)
  })

  it('file row has a hover class', async () => {
    await mountPicker()
    const row = await screen.findByTitle('Frieren_01.mkv')
    expect(row.closest('li')?.className).toMatch(/hover:bg-control/)
  })

  it('caps the files list so a long tree scrolls', async () => {
    getTorrentCandidateDetail.mockResolvedValue(
      detail({
        files: Array.from({ length: 20 }, (_, i) => ({
          name: `Frieren_${String(i + 1).padStart(2, '0')}.mkv`,
          size_bytes: 1_000_000,
        })),
      }),
    )
    await mountPicker()
    const heading = await screen.findByRole('heading', { name: 'Files' })
    expect(heading.nextElementSibling?.className).toMatch(/\bmax-h-80\b/)
    expect(heading.nextElementSibling?.className).toMatch(/\boverflow-y-auto\b/)
    expect(heading.nextElementSibling?.className).toMatch(/\bpr-3\b/)
  })
})
