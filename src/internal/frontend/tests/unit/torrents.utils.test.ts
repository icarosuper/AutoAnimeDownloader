import { describe, it, expect } from 'vitest'
import { formatSpeed, formatEta, formatPercent, totalSpeeds } from '../../src/lib/utils/torrents'
import type { TorrentInfo } from '../../src/lib/api/client'

describe('formatSpeed', () => {
  it('formats zero as an idle dash', () => {
    expect(formatSpeed(0)).toBe('—')
  })

  it('formats bytes per second', () => {
    expect(formatSpeed(512)).toBe('512 B/s')
  })

  it('formats kilobytes and megabytes with one decimal', () => {
    expect(formatSpeed(1536)).toBe('1.5 KB/s')
    expect(formatSpeed(2 * 1024 * 1024)).toBe('2.0 MB/s')
  })
})

describe('formatEta', () => {
  // ETA nulo = infinito/desconhecido. A rain só preenche ETA enquanto baixa com velocidade
  // maior que zero, então null é o caso comum, não a exceção.
  it('renders null as a dash', () => {
    expect(formatEta(null)).toBe('—')
  })

  it('renders seconds under a minute', () => {
    expect(formatEta(45)).toBe('45s')
  })

  it('renders minutes and seconds', () => {
    expect(formatEta(150)).toBe('2m 30s')
  })

  it('renders hours and minutes', () => {
    expect(formatEta(7380)).toBe('2h 3m')
  })

  it('renders days for very long ETAs', () => {
    expect(formatEta(180000)).toBe('2d 2h')
  })
})

describe('formatPercent', () => {
  it('renders a fraction as a whole percentage', () => {
    expect(formatPercent(0.4567)).toBe('46%')
  })

  it('renders zero progress', () => {
    expect(formatPercent(0)).toBe('0%')
  })

  it('renders full progress', () => {
    expect(formatPercent(1)).toBe('100%')
  })
})

describe('totalSpeeds', () => {
  const torrent = (download: number, upload: number): TorrentInfo => ({
    hash: 'h' + download,
    name: 'n',
    status: 'downloading',
    completed: false,
    episode_number: null,
    is_batch: false,
    bytes_completed: 0,
    bytes_total: 0,
    bytes_uploaded: 0,
    progress: 0,
    download_speed: download,
    upload_speed: upload,
    peers_total: 0,
    eta_seconds: null,
    seeded_for_seconds: 0,
  })

  it('sums download and upload across torrents', () => {
    expect(totalSpeeds([torrent(100, 10), torrent(250, 40)])).toEqual({ download: 350, upload: 50 })
  })

  it('returns zeros for an empty list', () => {
    expect(totalSpeeds([])).toEqual({ download: 0, upload: 0 })
  })
})
