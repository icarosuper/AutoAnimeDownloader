import { describe, it, expect } from 'vitest'
import { render, fireEvent, screen } from '@testing-library/svelte'
import ChipsInput from '../../src/components/ui/ChipsInput.svelte'

/**
 * The five keyboard rules from spec §9.4 (Enter, comma, Backspace on an empty field, ×,
 * paste a list). They are why the component exists: without them the field has no way to
 * receive a value, because the old design's "+" button was removed.
 *
 * The asserts read the rendered DOM (one × per chip, each with accessible name "Remove <v>"),
 * not the component's internal state: that is what the user and the screen reader see.
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

/** fireEvent.input does not change the value on its own; bind:value reads from the DOM. */
async function type(input: HTMLInputElement, text: string) {
  input.value = text
  await fireEvent.input(input)
}

describe('ChipsInput', () => {
  it('commits the value with Enter', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual(['icaro'])
    expect(input.value).toBe('')
  })

  it('commits the value with a comma', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.keyDown(input, { key: ',' })

    expect(chips()).toEqual(['icaro'])
    expect(input.value).toBe('')
  })

  it('removes the last chip with Backspace when the field is empty', async () => {
    const input = setup(['alpha', 'beta'])

    await fireEvent.keyDown(input, { key: 'Backspace' })

    expect(chips()).toEqual(['alpha'])
  })

  it('does NOT remove a chip with Backspace while there is typed text', async () => {
    const input = setup(['alpha'])

    await type(input, 'xy')
    await fireEvent.keyDown(input, { key: 'Backspace' })

    expect(chips()).toEqual(['alpha'])
  })

  it('removes the chip via ×', async () => {
    setup(['alpha', 'beta'])

    await fireEvent.click(screen.getByRole('button', { name: 'Remove alpha' }))

    expect(chips()).toEqual(['beta'])
  })

  it('creates several chips when pasting a comma-separated list', async () => {
    const input = setup()

    await fireEvent.paste(input, {
      clipboardData: { getData: () => 'alpha, beta ,gamma' },
    })

    expect(chips()).toEqual(['alpha', 'beta', 'gamma'])
    expect(input.value).toBe('')
  })

  it('does not duplicate a value that already exists', async () => {
    const input = setup(['alpha'])

    await type(input, 'alpha')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual(['alpha'])
  })

  it('ignores empty or whitespace-only input', async () => {
    const input = setup()

    await type(input, '   ')
    await fireEvent.keyDown(input, { key: 'Enter' })

    expect(chips()).toEqual([])
  })

  it('commits what was typed on leaving the field (does not lose the text on Save)', async () => {
    const input = setup()

    await type(input, 'icaro')
    await fireEvent.blur(input)

    expect(chips()).toEqual(['icaro'])
  })
})
