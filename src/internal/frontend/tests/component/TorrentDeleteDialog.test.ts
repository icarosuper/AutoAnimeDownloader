import { describe, it, expect } from 'vitest'
import { render, fireEvent } from '@testing-library/svelte'
import TorrentDeleteDialog from '../../src/components/TorrentDeleteDialog.svelte'

type ConfirmDetail = { keepData: boolean; block: boolean }

describe('TorrentDeleteDialog', () => {
  it('starts with both checkboxes checked', () => {
    const { getAllByRole } = render(TorrentDeleteDialog, { props: { open: true, count: 1, name: 'Frieren' } })
    const checkboxes = getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes).toHaveLength(2)
    expect(checkboxes[0].checked).toBe(true)
    expect(checkboxes[1].checked).toBe(true)
  })

  it('changes the consequence text when "don\'t download again" is unchecked', async () => {
    const { getAllByRole, getByText } = render(TorrentDeleteDialog, { props: { open: true, count: 1, name: 'Frieren' } })
    const checkboxes = getAllByRole('checkbox') as HTMLInputElement[]
    const blockCheckbox = checkboxes[1]

    // Checked by default: no re-download warning.
    expect(getByText(/will not be downloaded again/i)).toBeInTheDocument()

    await fireEvent.click(blockCheckbox)

    expect(getByText(/may download this episode again/i)).toBeInTheDocument()
  })

  it('emits confirm with { keepData: false, block: true } when both checkboxes stay checked', async () => {
    let detail: ConfirmDetail | null = null
    const { getAllByRole } = render(TorrentDeleteDialog, {
      props: { open: true, count: 1, name: 'Frieren' },
      events: { confirm: (e: CustomEvent<ConfirmDetail>) => { detail = e.detail } },
    })

    const confirmButton = getAllByRole('button').find((b) => b.textContent?.trim() === 'Delete')
    expect(confirmButton).toBeTruthy()
    await fireEvent.click(confirmButton as HTMLElement)

    expect(detail).toEqual({ keepData: false, block: true })
  })

  it('emits confirm with { keepData: true, block: false } when both checkboxes are unchecked', async () => {
    let detail: ConfirmDetail | null = null
    const { getAllByRole } = render(TorrentDeleteDialog, {
      props: { open: true, count: 1, name: 'Frieren' },
      events: { confirm: (e: CustomEvent<ConfirmDetail>) => { detail = e.detail } },
    })
    const checkboxes = getAllByRole('checkbox') as HTMLInputElement[]
    await fireEvent.click(checkboxes[0])
    await fireEvent.click(checkboxes[1])

    const confirmButton = getAllByRole('button').find((b) => b.textContent?.trim() === 'Delete')
    await fireEvent.click(confirmButton as HTMLElement)

    expect(detail).toEqual({ keepData: true, block: false })
  })

  it('emits confirm with { keepData: false, block: false } when only "don\'t download again" is unchecked', async () => {
    let detail: ConfirmDetail | null = null
    const { getAllByRole } = render(TorrentDeleteDialog, {
      props: { open: true, count: 1, name: 'Frieren' },
      events: { confirm: (e: CustomEvent<ConfirmDetail>) => { detail = e.detail } },
    })
    const checkboxes = getAllByRole('checkbox') as HTMLInputElement[]
    await fireEvent.click(checkboxes[1])

    const confirmButton = getAllByRole('button').find((b) => b.textContent?.trim() === 'Delete')
    await fireEvent.click(confirmButton as HTMLElement)

    expect(detail).toEqual({ keepData: false, block: false })
  })
})
