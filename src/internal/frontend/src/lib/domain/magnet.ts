// Mirrors torrents.parseInfoHash: a magnet is valid when it has an xt=urn:btih:
// of 40 hex chars or 32 uppercase base32 chars. v2-only (btmh) and short hashes
// stay rejected so the picker button matches what the daemon will accept.

const BTIH = 'urn:btih:'

export function isValidMagnet(raw: string): boolean {
  const trimmed = raw.trim()
  let u: URL
  try {
    u = new URL(trimmed)
  } catch {
    return false
  }
  if (u.protocol.toLowerCase() !== 'magnet:') return false

  for (const xt of u.searchParams.getAll('xt')) {
    if (!xt.startsWith(BTIH)) continue
    const hash = xt.slice(BTIH.length)
    if (hash.length === 40 && /^[0-9a-fA-F]+$/.test(hash)) return true
    if (hash.length === 32 && /^[A-Z2-7]+$/.test(hash)) return true
  }
  return false
}
