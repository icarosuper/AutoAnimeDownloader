import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte'
import Notifications from '../../src/routes/Notifications.svelte'
import * as client from '../../src/lib/api/client.js'
import type { Config, WebhookPreset } from '../../src/lib/api/client.js'

vi.mock('../../src/lib/api/client.js', () => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  testWebhook: vi.fn(),
}))

const getConfig = vi.mocked(client.getConfig)
const updateConfig = vi.mocked(client.updateConfig)

const config = (webhooks: WebhookPreset[] = []) =>
  ({ notifications: { webhooks, batch_window_seconds: 0 } }) as Config

/** Buttons and labels change with the jsdom locale; placeholders are literals in the markup. */
const btn = (re: RegExp) => screen.getByRole('button', { name: re })
const ADD = /Adicionar webhook|Add webhook/
const CONFIRM = /Confirmar|Confirm/
const SAVE = /^(Salvar|Save)$/

async function renderWith(webhooks: WebhookPreset[] = []) {
  getConfig.mockResolvedValue(config(webhooks))
  render(Notifications)
  await waitFor(() => expect(getConfig).toHaveBeenCalled())
}

async function savedWebhooks(): Promise<WebhookPreset[]> {
  await waitFor(() => expect(updateConfig).toHaveBeenCalled())
  return (updateConfig.mock.calls[0][0] as Config).notifications.webhooks
}

beforeEach(() => vi.clearAllMocks())

describe('Notifications — header editor', () => {
  // The regression that motivated the change: typing in both fields and clicking Confirm
  // silently dropped the header, because only the "+" button passed it into the model.
  // See decisions.md #87.
  it('saves the typed header without going through the + button', async () => {
    await renderWith()
    await fireEvent.click(btn(ADD))

    await fireEvent.input(screen.getByPlaceholderText('e.g. ntfy'), { target: { value: 'Jellyfin' } })
    await fireEvent.input(screen.getByPlaceholderText(/^https:\/\/ntfy/), {
      target: { value: 'http://localhost:8096/Library/Refresh' },
    })
    await fireEvent.input(screen.getByPlaceholderText('Header'), { target: { value: 'X-Emby-Token' } })
    await fireEvent.input(screen.getByPlaceholderText('Value'), { target: { value: 'real-key' } })

    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({ 'X-Emby-Token': 'real-key' })
  })

  it('discards the blank row instead of saving a nameless header', async () => {
    await renderWith()
    await fireEvent.click(btn(ADD))

    await fireEvent.input(screen.getByPlaceholderText('e.g. ntfy'), { target: { value: 'Plex' } })
    await fireEvent.input(screen.getByPlaceholderText(/^https:\/\/ntfy/), { target: { value: 'http://x/y' } })

    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({})
  })

  it('opens an already-saved header inside an editable input', async () => {
    await renderWith([
      {
        name: 'Jellyfin',
        url: 'http://localhost:8096/Library/Refresh',
        method: 'POST',
        headers: { 'X-Emby-Token': 'old-key' },
        body: '',
        events: ['download_completed'],
      },
    ])
    await fireEvent.click(btn(/Editar|Edit/))

    const value = screen.getByPlaceholderText('Value') as HTMLInputElement
    expect(value.value).toBe('old-key')

    await fireEvent.input(value, { target: { value: 'new-key' } })
    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({ 'X-Emby-Token': 'new-key' })
  })
})
