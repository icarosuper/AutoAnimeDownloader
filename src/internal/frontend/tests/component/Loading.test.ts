import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import Loading from '../../src/components/Loading.svelte'

describe('Loading', () => {
  it('renders message prop', () => {
    const { getByText } = render(Loading, { props: { message: 'Please wait...' } })
    expect(getByText('Please wait...')).toBeInTheDocument()
  })

  it('renders spinner element', () => {
    const { container } = render(Loading, {})
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('uses default message when none provided', () => {
    const { getByText } = render(Loading, {})
    expect(getByText('Loading...')).toBeInTheDocument()
  })
})
