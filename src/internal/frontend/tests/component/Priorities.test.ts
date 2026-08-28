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

/** As sete listas menos criteria_order, a única sem campo de adicionar. */
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

/** Renderiza com o config do usuário divergindo (ou não) dos defaults do backend. */
async function setup(userPriorities: P = priorities()) {
  getConfig.mockResolvedValue({ priorities: userPriorities } as Config)
  getPriorityDefaults.mockResolvedValue(priorities())
  render(Priorities)
  await waitFor(() => expect(getConfig).toHaveBeenCalled())
  await screen.findByText('Codec')
}

beforeEach(() => vi.clearAllMocks())

describe('Priorities', () => {
  // Desmarcar não some da tela: token fora da lista já ranqueia como o pior no backend,
  // então ele continua visível, desmarcado, e volta com um clique.
  it('mostra o token canônico ausente da lista do usuário, desmarcado', async () => {
    await setup(priorities({ codecs: ['h.264', 'hevc'] }))

    // O glyph "✓" do Checkbox entra no textContent do <label>, daí o matcher por regex.
    expect((screen.getByLabelText(/Usar av1$/) as HTMLInputElement).checked).toBe(false)
    expect((screen.getByLabelText(/Usar h\.264$/) as HTMLInputElement).checked).toBe(true)
  })

  it('marcar/desmarcar entra e sai da lista salva', async () => {
    await setup(priorities({ codecs: ['h.264', 'hevc', 'av1', 'xvid'] }))

    await fireEvent.click(screen.getByLabelText(/Usar av1$/))
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(updateConfig).toHaveBeenCalled())
    expect(updateConfig.mock.calls[0][0].priorities.codecs).toEqual(['h.264', 'hevc', 'xvid'])
  })

  // ✕ só no que o usuário criou (inclui o legado inerte de um config.json antigo, como "x265").
  it('só oferece remover item que não está no default', async () => {
    await setup(priorities({ codecs: ['h.264', 'x265'] }))

    expect(screen.getByLabelText('Remover x265')).toBeTruthy()
    expect(screen.queryByLabelText('Remover h.264')).toBeNull()
  })

  // criteria_order é conjunto fechado (o backend pula em silêncio o critério que não conhece) e
  // os oito já aparecem sempre, marcados ou não: não há campo de adicionar ali.
  it('ordem dos critérios não tem campo de adicionar, e mostra o critério que falta desmarcado', async () => {
    await setup(priorities({ criteria_order: ['resolution', 'fansub'] }))

    expect(screen.getAllByPlaceholderText('Adicionar item')).toHaveLength(LISTS_WITH_ADD)
    expect((screen.getByLabelText(/Usar codec$/) as HTMLInputElement).checked).toBe(false)
    expect((screen.getByLabelText(/Usar resolution$/) as HTMLInputElement).checked).toBe(true)
  })

  it('preset de codec promove só o que já está na lista', async () => {
    await setup(priorities({ codecs: ['hevc', 'av1', 'h.264', 'xvid'] }))

    await fireEvent.click(screen.getByRole('button', { name: 'Prefiro compatibilidade' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Salvar' }))

    await waitFor(() => expect(updateConfig).toHaveBeenCalled())
    expect(updateConfig.mock.calls[0][0].priorities.codecs).toEqual(['h.264', 'hevc', 'av1', 'xvid'])
  })
})
