import { describe, it, expect } from 'vitest'
import { render, within } from '@testing-library/svelte'
import AnimeAccountsPanel from '../../src/components/AnimeAccountsPanel.svelte'
import type { AnimeListAccount } from '../../src/lib/api/client.js'

const label = (s: string) => ({ CURRENT: 'Watching', PLANNING: 'Planning', COMPLETED: 'Completed' })[s] ?? s

describe('AnimeAccountsPanel', () => {
  const accounts: AnimeListAccount[] = [
    { source: 'anilist', username: 'alice', status: 'COMPLETED', progress: 12, score: 8.5, score_raw: 85, score_format: 'POINT_100', avatar: 'http://a/av.png' },
    { source: 'anilist', username: 'bob', status: 'PLANNING', progress: 0 },
    { source: 'mal', username: 'carol', status: 'CURRENT', progress: 3, score: 7, score_raw: 7, score_format: 'POINT_10' },
    { source: 'anilist', username: 'dave', status: 'COMPLETED', progress: 12, score: 8.5, score_raw: 3, score_format: 'POINT_3' },
  ]

  it('renders nothing without accounts', () => {
    const { container } = render(AnimeAccountsPanel, { props: { accounts: [], statusLabel: label } })
    expect(container.querySelector('section')).toBeNull()
  })

  it('shows each account with its own status, progress, score in its own format, and a profile link', () => {
    const { getAllByRole } = render(AnimeAccountsPanel, { props: { accounts, statusLabel: label, totalEpisodes: 12 } })
    const rows = getAllByRole('listitem')
    expect(rows).toHaveLength(4)

    const [alice, bob, carol, dave] = rows.map(r => within(r))
    expect(alice.getByText(/Completed/)).toBeTruthy()
    expect(alice.getByText(/ep 12\/12/)).toBeTruthy()
    expect(alice.getByText('85/100')).toBeTruthy()
    expect(alice.getByRole('link').getAttribute('href')).toBe('https://anilist.co/user/alice/animelist')
    expect(rows[0].querySelector('img')?.getAttribute('src')).toBe('http://a/av.png')

    expect(bob.getByText(/Planning/)).toBeTruthy()
    expect(rows[1].textContent).not.toMatch(/ep |\/10/)

    expect(carol.getByText('MAL')).toBeTruthy()
    expect(carol.getByText('7/10')).toBeTruthy()
    expect(carol.getByText(/ep 3\/12/)).toBeTruthy()
    expect(carol.getByRole('link').getAttribute('href')).toBe('https://myanimelist.net/animelist/carol')

    expect(dave.getByRole('img', { name: 'Liked it' })).toBeTruthy()
  })
})
