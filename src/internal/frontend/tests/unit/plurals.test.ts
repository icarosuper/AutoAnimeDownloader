import { describe, it, expect } from 'vitest'
import * as m from '../../src/lib/i18n/messages.js'

// Count messages pick the singular with an exact match on 1 (`count=1` / `count=*` in
// messages/*.json), not with `: plural`: CLDR puts 0 in "one" for Portuguese, so the plural
// rule would print "0 visto". These pin both ends of that choice.
const forms = (fn: (i: { count: number }, o: { locale: 'en' | 'pt-BR' }) => string, locale: 'en' | 'pt-BR') =>
  [0, 1, 2].map((count) => fn({ count }, { locale }))

describe('count messages', () => {
  it('singular only for exactly one, in Portuguese too', () => {
    expect(forms(m.progress_legend_watched, 'pt-BR')).toEqual(['0 vistos', '1 visto', '2 vistos'])
    expect(forms(m.progress_legend_unreleased, 'pt-BR')).toEqual(['0 não lançados', '1 não lançado', '2 não lançados'])
    expect(forms(m.status_animes_count, 'en')).toEqual(['0 animes', '1 anime', '2 animes'])
  })

  it('replaces the "(s)" hedges', () => {
    expect(forms(m.downloads_group_count, 'en')).toEqual(['0 torrents', '1 torrent', '2 torrents'])
    expect(m.detail_bulk_confirm_msg({ count: 1 }, { locale: 'pt-BR' })).toBe(
      '1 episódio será removido. Esta ação não pode ser desfeita.',
    )
  })

  it('selects each count on its own when a message has two', () => {
    expect(m.lastcheck_no_seeders({ candidates: 1, seeders: 3 }, { locale: 'en' })).toBe(
      '1 torrent found, without at least 3 seeders.',
    )
    expect(m.lastcheck_no_seeders({ candidates: 4, seeders: 1 }, { locale: 'pt-BR' })).toBe(
      '4 torrents encontrados, nenhum com pelo menos 1 seeder.',
    )
  })
})
