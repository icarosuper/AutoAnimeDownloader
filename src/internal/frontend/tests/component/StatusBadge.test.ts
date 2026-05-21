import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/svelte'
import StatusBadge from '../../src/components/StatusBadge.svelte'

describe('StatusBadge', () => {
  it('shows "Running" for status=running', () => {
    const { getByText } = render(StatusBadge, { props: { status: 'running' } })
    expect(getByText('Running')).toBeInTheDocument()
  })

  it('shows "Stopped" for status=stopped', () => {
    const { getByText } = render(StatusBadge, { props: { status: 'stopped' } })
    expect(getByText('Stopped')).toBeInTheDocument()
  })

  it('shows "Checking" for status=checking', () => {
    const { getByText } = render(StatusBadge, { props: { status: 'checking' } })
    expect(getByText('Checking')).toBeInTheDocument()
  })

  it('applies green color class for running status', () => {
    const { container } = render(StatusBadge, { props: { status: 'running' } })
    expect(container.querySelector('span')?.className).toContain('bg-green')
  })

  it('applies red color class for stopped status', () => {
    const { container } = render(StatusBadge, { props: { status: 'stopped' } })
    expect(container.querySelector('span')?.className).toContain('bg-red')
  })
})
