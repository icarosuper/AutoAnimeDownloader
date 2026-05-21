import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import ErrorMessage from '../../src/components/ErrorMessage.svelte'

describe('ErrorMessage', () => {
  it('renders the message prop', () => {
    const { getByText } = render(ErrorMessage, { props: { message: 'Something went wrong' } })
    expect(getByText('Something went wrong')).toBeInTheDocument()
  })

  it('uses default message when none provided', () => {
    const { getByText } = render(ErrorMessage, {})
    expect(getByText('An error occurred')).toBeInTheDocument()
  })
})
