import { describe, it, expect } from 'vitest'
import { isValidMagnet } from '../../src/lib/domain/magnet'

const HEX = '0123456789abcdef0123456789abcdef01234567'
const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

describe('isValidMagnet', () => {
  it('accepts a hex btih magnet, extra params, surrounding space and MAGNET: scheme', () => {
    expect(isValidMagnet(`magnet:?xt=urn:btih:${HEX}`)).toBe(true)
    expect(isValidMagnet(`magnet:?xt=urn:btih:${HEX}&dn=Show`)).toBe(true)
    expect(isValidMagnet(`  MAGNET:?xt=urn:btih:${HEX}  `)).toBe(true)
  })

  it('accepts uppercase base32 btih', () => {
    expect(isValidMagnet(`magnet:?xt=urn:btih:${B32}`)).toBe(true)
  })

  it('rejects empty, http, missing xt, short hash, lowercase base32 and v2-only', () => {
    expect(isValidMagnet('')).toBe(false)
    expect(isValidMagnet('   ')).toBe(false)
    expect(isValidMagnet('http://nyaa.si/view/1')).toBe(false)
    expect(isValidMagnet('magnet:?dn=NoXT')).toBe(false)
    expect(isValidMagnet('magnet:?xt=urn:btih:pasted')).toBe(false)
    expect(isValidMagnet(`magnet:?xt=urn:btih:${B32.toLowerCase()}`)).toBe(false)
    expect(isValidMagnet(`magnet:?xt=urn:btmh:1220${'aa'.repeat(32)}`)).toBe(false)
  })
})
