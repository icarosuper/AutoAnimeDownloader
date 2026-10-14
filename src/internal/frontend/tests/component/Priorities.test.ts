import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Priorities from '../../src/routes/Priorities.svelte'
import * as client from '../../src/lib/api/client.js'
import type { Config, Priorities as P } from '../../src/lib/api/client.js'

vi.mock('../../src/lib/api/client.js', () => ({
  getConfig: vi.fn(),
  getPriorityDefaults: vi.fn(),
  updateConfig: vi.fn(),
}))

const getConfig = vi.mocked(client.getConfig)
const getPriorityDefaults = vi.mocked(client.getPriorityDefaults)
const updateConfig = vi.mocked(client.updateConfig)

/** The seven lists minus criteria_order, the only one without an add field. */
const LISTS_WITH_ADD = 6

const CRITERIA = ['uncensored', 'source', 'resolution', 'health', 'codec', 'fansub', 'audio', 'size']

function priorities(over: Partial<P> = {}): P {
  return {
    criteria_order: CRITERIA,
    fansubs: ['subsplease', 'erai-raws'],
    resolutions: ['1080p', '720p'],
    sources: ['bdremux'],
    codecs: ['h.264', 'hevc', 'av1', 'xvid'],
    audio: ['flac'],
    ignore_list: ['[dub]'],
    ...over,
  }
}

/** Renders with the user's config diverging (or not) from the backend defaults. jsdom's locale is en. */
async function setup(userPriorities: P = priorities()) {
  getConfig.mockResolvedValue({ priorities: userPriorities } as Config)
  getPriorityDefaults.mockResolvedValue(priorities())
  render(Priorities)
  await waitFor(() => expect(getConfig).toHaveBeenCalled())
  await screen.findByText('Codec')
}

beforeEach(() => vi.clearAllMocks())

describe('Priorities', () => {
  // Unchecking doesn't remove it from the screen: a token outside the list already ranks as the
  // worst in the backend, so it stays visible, unchecked, and comes back with one click.
  it("shows the canonical token missing from the user's list, unchecked", async () => {
    await setup(priorities({ codecs: ['h.264', 'hevc'] }))

    // The Checkbox's "✓" glyph is part of the <label>'s textContent, hence the regex matcher.
    expect((screen.getByLabelText(/Use av1$/) as HTMLInputElement).checked).toBe(false)
    expect((screen.getByLabelText(/Use h\.264$/) as HTMLInputElement).checked).toBe(true)
  })

  it('checking/unchecking adds to and removes from the saved list', async () => {
    await setup(priorities({ codecs: ['h.264', 'hevc', 'av1', 'xvid'] }))

    await fireEvent.click(screen.getByLabelText(/Use av1$/))
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(updateConfig).toHaveBeenCalled())
    expect(updateConfig.mock.calls[0][0].priorities.codecs).toEqual(['h.264', 'hevc', 'xvid'])
  })

  // ✕ only on what the user created (including the inert leftovers of an old config.json, like "x265").
  it('only offers to remove an item that is not in the default', async () => {
    await setup(priorities({ codecs: ['h.264', 'x265'] }))

    expect(screen.getByLabelText('Remove x265')).toBeTruthy()
    expect(screen.queryByLabelText('Remove h.264')).toBeNull()
  })

  // criteria_order is a closed set (the backend silently skips a criterion it doesn't know) and
  // all eight always show, checked or not: there's no add field there.
  it('criteria order has no add field, and shows the missing criterion unchecked', async () => {
    await setup(priorities({ criteria_order: ['resolution', 'fansub'] }))

    expect(screen.getAllByPlaceholderText('Add item')).toHaveLength(LISTS_WITH_ADD)
    expect((screen.getByLabelText(/Use codec$/) as HTMLInputElement).checked).toBe(false)
    expect((screen.getByLabelText(/Use resolution$/) as HTMLInputElement).checked).toBe(true)
  })

  it('the codec preset only promotes what is already in the list', async () => {
    await setup(priorities({ codecs: ['hevc', 'av1', 'h.264', 'xvid'] }))

    await fireEvent.click(screen.getByRole('button', { name: 'I prefer compatibility' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => expect(updateConfig).toHaveBeenCalled())
    expect(updateConfig.mock.calls[0][0].priorities.codecs).toEqual(['h.264', 'hevc', 'av1', 'xvid'])
  })
})
