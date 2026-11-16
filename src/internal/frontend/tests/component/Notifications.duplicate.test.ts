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

const btn = (re: RegExp) => screen.getByRole('button', { name: re })
const CONFIRM = /Confirmar|Confirm/
const SAVE = /^(Salvar|Save)$/
const NAME = 'e.g. ntfy'

const ntfy: WebhookPreset = {
  name: 'ntfy',
  url: 'https://ntfy.sh/episodes',
  method: 'POST',
  headers: { Authorization: 'Bearer secret' },
  body: '{{message}}',
  events: ['download_completed'],
  anime_sources: ['anilist:alice'],
}

async function renderWith(webhooks: WebhookPreset[]) {
  getConfig.mockResolvedValue({ notifications: { webhooks, batch_window_seconds: 0 } } as Config)
  render(Notifications)
  await waitFor(() => expect(getConfig).toHaveBeenCalled())
}

async function savedWebhooks(): Promise<WebhookPreset[]> {
  await waitFor(() => expect(updateConfig).toHaveBeenCalled())
  return (updateConfig.mock.calls[0][0] as Config).notifications.webhooks
}

beforeEach(() => vi.clearAllMocks())

describe('Notifications — duplicate', () => {
  it('adds a copy with every setting and leaves the original untouched', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Duplicar|Duplicate)$/))

    const name = screen.getByPlaceholderText(NAME) as HTMLInputElement
    expect(name.value).toMatch(/^ntfy \((cópia|copy)\)$/)
    expect((screen.getByPlaceholderText('Value') as HTMLInputElement).value).toBe('Bearer secret')

    await fireEvent.input(name, { target: { value: 'ntfy alerts' } })
    await fireEvent.input(screen.getByPlaceholderText('Value'), { target: { value: 'Bearer other' } })
    await fireEvent.click(btn(CONFIRM))
    await fireEvent.click(btn(SAVE))

    const saved = await savedWebhooks()
    expect(saved).toHaveLength(2)
    expect(saved[0]).toEqual(ntfy)
    expect(saved[1]).toEqual({ ...ntfy, name: 'ntfy alerts', headers: { Authorization: 'Bearer other' } })
  })

  it('cancel adds nothing', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Duplicar|Duplicate)$/))
    await fireEvent.click(btn(/^(Cancelar|Cancel)$/))
    await fireEvent.click(btn(SAVE))

    expect(await savedWebhooks()).toEqual([ntfy])
  })
})

describe('Notifications — name already taken', () => {
  it('blocks Confirm while the name matches another webhook', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Duplicar|Duplicate)$/))

    await fireEvent.input(screen.getByPlaceholderText(NAME), { target: { value: ' ntfy ' } })
    expect((btn(CONFIRM) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText(/ntfy/, { selector: '#webhook-name-error' })).toBeTruthy()

    await fireEvent.input(screen.getByPlaceholderText(NAME), { target: { value: 'ntfy 2' } })
    expect((btn(CONFIRM) as HTMLButtonElement).disabled).toBe(false)
    expect(document.querySelector('#webhook-name-error')).toBeNull()
  })

  it('editing a webhook can keep its own name', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Editar|Edit)$/))

    expect((btn(CONFIRM) as HTMLButtonElement).disabled).toBe(false)
    expect(document.querySelector('#webhook-name-error')).toBeNull()
  })
})

describe('Notifications — unsaved changes', () => {
  const dirty = async () => {
    const { get } = await import('svelte/store')
    const { hasUnsavedChanges } = await import('../../src/lib/stores/unsavedChanges.js')
    return get(hasUnsavedChanges)
  }

  it('opening the form untouched is not dirty; typing is; typing it back is not', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Editar|Edit)$/))
    expect(await dirty()).toBe(false)

    const url = screen.getByPlaceholderText(/^https:\/\/ntfy/)
    await fireEvent.input(url, { target: { value: 'https://ntfy.sh/other' } })
    expect(await dirty()).toBe(true)
    await fireEvent.input(url, { target: { value: ntfy.url } })
    expect(await dirty()).toBe(false)
  })

  it('a confirmed but unsaved change is dirty until Save', async () => {
    await renderWith([ntfy])
    await fireEvent.click(btn(/^(Remover|Remove)$/))
    expect(await dirty()).toBe(true)

    await fireEvent.click(btn(SAVE))
    await waitFor(async () => expect(await dirty()).toBe(false))
  })
})
