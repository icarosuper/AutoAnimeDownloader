import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import UpdateBadge from '../../src/components/shell/UpdateBadge.svelte'

describe('UpdateBadge', () => {
  it('links to the release page when a newer version exists', () => {
    render(UpdateBadge, { version: '2.11.0', url: 'https://example.invalid/v2.11.0' })
    const link = screen.getByRole('link', { name: /v2\.11\.0 (available|disponível)/ })
    expect(link.getAttribute('href')).toBe('https://example.invalid/v2.11.0')
    expect(link.getAttribute('target')).toBe('_blank')
  })

  it('renders nothing without a newer version', () => {
    render(UpdateBadge, { version: '', url: '' })
    expect(screen.queryByRole('link')).toBeNull()
  })
})
