import { describe, it, expect } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/svelte'
import ChipsInput from '../../src/components/ui/ChipsInput.svelte'

/**
 * As cinco regras de teclado do spec §9.4 (Enter, vírgula, Backspace em campo vazio, ×, colar
 * lista). São a razão de o componente existir: sem elas o campo não tem como receber valor,
 * porque o botão "+" do design antigo foi removido.
 *
 * Os asserts leem o DOM renderizado (um × por chip, cada um com nome acessível "Remove <v>"),
 * não o estado interno do componente: é o mesmo que o usuário e o leitor de tela enxergam.
 */
function setup(values: string[] = []) {
  render(ChipsInput, { props: { label: 'Usernames', values } })
  return screen.getByLabelText('Usernames') as HTMLInputElement
}

function chips(): string[] {
  return screen
    .queryAllByRole('button', { name: /^Remove / })
    .map((btn) => btn.getAttribute('aria-label')!.replace(/^Remove /, ''))
}

/** fireEvent.input não mexe no valor sozinho; o bind:value lê do DOM. */
async function type(input: HTMLInputElement, text: string) {
  input.value = text
  await fireEvent.input(input)
}

describe('ChipsInput', () => {
  it('confirma o valor com Enter', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual(['icaro'])
    expect(input.value).toBe('')
  })

  it('confirma o valor com vírgula', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.keyDown(input, { key: ',' })

    expect(chips()).toEqual(['icaro'])
    expect(input.value).toBe('')
  })

  it('remove o último chip com Backspace quando o campo está vazio', async () => {
    const input = setup(['alpha', 'beta'])

    await fireEvent.keyDown(input, { key: 'Backspace' })

    expect(chips()).toEqual(['alpha'])
  })

  it('NÃO remove chip com Backspace enquanto há texto digitado', async () => {
    const input = setup(['alpha'])

    await type(input, 'xy')
    await fireEvent.keyDown(input, { key: 'Backspace' })

    expect(chips()).toEqual(['alpha'])
  })

  it('remove o chip pelo ×', async () => {
    setup(['alpha', 'beta'])

    await fireEvent.click(screen.getByRole('button', { name: 'Remove alpha' }))

    expect(chips()).toEqual(['beta'])
  })

  it('cria vários chips ao colar uma lista separada por vírgulas', async () => {
    const input = setup()

    await fireEvent.paste(input, {
      clipboardData: { getData: () => 'alpha, beta ,gamma' },
    })

    expect(chips()).toEqual(['alpha', 'beta', 'gamma'])
    expect(input.value).toBe('')
  })

  it('não duplica um valor que já existe', async () => {
    const input = setup(['alpha'])

    await type(input, 'alpha')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual(['alpha'])
  })

  it('ignora entrada vazia ou só com espaços', async () => {
    const input = setup()

    await type(input, '   ')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual([])
  })

  it('confirma o que estava digitado ao sair do campo (não perde o texto no Salvar)', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.blur(input)

    expect(chips()).toEqual(['icaro'])
  })
})
