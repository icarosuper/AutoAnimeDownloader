import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import VersionLink from '../../src/components/shell/VersionLink.svelte'
import { REPO_URL } from '../../src/lib/report'

describe('VersionLink', () => {
  it('shows the version as a link to the repository in a new tab', () => {
    render(VersionLink, { version: '2.11.4' })
    const link = screen.getByRole('link', { name: /v2\.11\.4/ })
    expect(link.getAttribute('href')).toBe(REPO_URL)
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toContain('noopener')
  })
})
