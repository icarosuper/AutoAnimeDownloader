import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Notifications from '../../src/routes/Notifications.svelte'
import * as client from '../../src/lib/api/client.js'
import type { Config, WebhookPreset } from '../../src/lib/api/client.js'

vi.mock('../../src/lib/api/client.js', () => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  testWebhook: vi.fn(),
  previewWebhook: vi.fn(),
}))

const getConfig = vi.mocked(client.getConfig)
const updateConfig = vi.mocked(client.updateConfig)

const hook: WebhookPreset = {
  name: 'ntfy', url: 'https://ntfy.sh/x', method: 'POST', headers: {}, body: '{{message}}', events: ['download_completed'],
}

async function renderWith(webhooks: WebhookPreset[]) {
  getConfig.mockResolvedValue({
    anilist_usernames: ['alice', 'bob'],
    mal_usernames: [],
    notifications: { webhooks, batch_window_seconds: 0 },
  } as unknown as Config)
  render(Notifications)
  await waitFor(() => expect(getConfig).toHaveBeenCalled())
  await fireEvent.click(await screen.findByRole('button', { name: /^(Editar|Edit)$/ }))
}

async function saved(): Promise<WebhookPreset> {
  await fireEvent.click(screen.getByRole('button', { name: /Confirmar|Confirm/ }))
  await fireEvent.click(screen.getByRole('button', { name: /^(Salvar|Save)$/ }))
  await waitFor(() => expect(updateConfig).toHaveBeenCalled())
  return (updateConfig.mock.calls[0][0] as Config).notifications.webhooks[0]
}

const box = (name: RegExp) => screen.getByRole('checkbox', { name }) as HTMLInputElement

beforeEach(() => vi.clearAllMocks())

describe('Notifications — which animes', () => {
  it('starts on "All animes", locked, with every account and Standalone unchecked', async () => {
    await renderWith([hook])
    expect(box(/Todos os animes|All animes/).checked).toBe(true)
    expect(box(/Todos os animes|All animes/).disabled).toBe(true)
    expect(box(/^alice$/).checked).toBe(false)
    expect(box(/^(Avulsos|Standalone)$/).checked).toBe(false)
  })

  it('saves the checked accounts and unlocks "All"', async () => {
    await renderWith([hook])
    await fireEvent.click(box(/^bob$/))
    await fireEvent.click(box(/^(Avulsos|Standalone)$/))
    expect(box(/Todos os animes|All animes/).checked).toBe(false)
    expect((await saved()).anime_sources).toEqual(['anilist:bob', 'standalone'])
  })

  it('"All" clears the filter', async () => {
    await renderWith([{ ...hook, anime_sources: ['anilist:alice'] }])
    expect(box(/^alice$/).checked).toBe(true)
    await fireEvent.click(box(/Todos os animes|All animes/))
    expect((await saved()).anime_sources).toEqual([])
  })

  it('shows a source whose account is gone, so it can be unchecked', async () => {
    await renderWith([{ ...hook, anime_sources: ['anilist:gone'] }])
    await fireEvent.click(box(/anilist:gone/))
    expect((await saved()).anime_sources).toEqual([])
  })
})
