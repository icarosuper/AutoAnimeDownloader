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

/** Botões e labels mudam com o locale do jsdom; os placeholders são literais no markup. */
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

describe('Notifications — editor de headers', () => {
  // A regressão que motivou a mudança: digitar nos dois campos e clicar em Confirmar perdia o
  // header em silêncio, porque só o botão "+" o passava para o modelo. Ver decisions.md #87.
  it('grava o header digitado sem passar pelo botão +', async () => {
    await renderWith()
    await fireEvent.click(btn(ADD))

    await fireEvent.input(screen.getByPlaceholderText('ex: ntfy'), { target: { value: 'Jellyfin' } })
    await fireEvent.input(screen.getByPlaceholderText(/^https:\/\/ntfy/), {
      target: { value: 'http://localhost:8096/Library/Refresh' },
    })
    await fireEvent.input(screen.getByPlaceholderText('Header'), { target: { value: 'X-Emby-Token' } })
    await fireEvent.input(screen.getByPlaceholderText('Value'), { target: { value: 'chave-real' } })

    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({ 'X-Emby-Token': 'chave-real' })
  })

  it('descarta a linha em branco em vez de gravar um header sem nome', async () => {
    await renderWith()
    await fireEvent.click(btn(ADD))

    await fireEvent.input(screen.getByPlaceholderText('ex: ntfy'), { target: { value: 'Plex' } })
    await fireEvent.input(screen.getByPlaceholderText(/^https:\/\/ntfy/), { target: { value: 'http://x/y' } })

    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({})
  })

  it('abre o header já salvo dentro de um input, editável', async () => {
    await renderWith([
      {
        name: 'Jellyfin',
        url: 'http://localhost:8096/Library/Refresh',
        method: 'POST',
        headers: { 'X-Emby-Token': 'chave-velha' },
        body: '',
        events: ['download_completed'],
      },
    ])
    await fireEvent.click(btn(/Editar|Edit/))

    const value = screen.getByPlaceholderText('Value') as HTMLInputElement
    expect(value.value).toBe('chave-velha')

    await fireEvent.input(value, { target: { value: 'chave-nova' } })
    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    expect((await savedWebhooks())[0].headers).toEqual({ 'X-Emby-Token': 'chave-nova' })
  })
})
