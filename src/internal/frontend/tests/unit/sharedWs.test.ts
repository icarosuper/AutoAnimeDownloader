import { describe, it, expect, vi, beforeEach } from 'vitest'
import { get } from 'svelte/store'

// Each constructed client is recorded with the callbacks it was given, so a test can play the
// socket's side: report a state, push a status, or fire a late close from a replaced client.
type Recorded = {
  statusCb: (status: string, lastCheck: string, hasError: boolean) => void
  connCb: (state: 'connected' | 'reconnecting' | 'disconnected') => void
  disconnected: boolean
}
const clients: Recorded[] = []

vi.mock('../../src/lib/websocket/client.js', () => ({
  WebSocketClient: class {
    rec!: Recorded
    connect(statusCb: Recorded['statusCb'], connCb: Recorded['connCb']) {
      this.rec = { statusCb, connCb, disconnected: false }
      clients.push(this.rec)
    }
    disconnect() {
      this.rec.disconnected = true
    }
  },
}))

const { ensureWsConnected, reconnectWs, onDaemonStatus, resetSharedWs } = await import(
  '../../src/lib/websocket/shared.js'
)
const { wsConnectionState } = await import('../../src/lib/stores/wsState.js')

beforeEach(() => {
  resetSharedWs()
  clients.length = 0
})

describe('shared WebSocket', () => {
  it('starts as connecting, not disconnected', () => {
    // A red dot before the first answer was the bug: every page load showed "Disconnected".
    expect(get(wsConnectionState)).toBe('connecting')
  })

  it('opens one connection no matter how many screens ask', () => {
    ensureWsConnected()
    ensureWsConnected()
    clients[0].connCb('connected')
    ensureWsConnected()
    expect(clients).toHaveLength(1)
    expect(get(wsConnectionState)).toBe('connected')
  })

  it('does not reopen while the client is still retrying', () => {
    ensureWsConnected()
    clients[0].connCb('reconnecting')
    ensureWsConnected()
    expect(clients).toHaveLength(1)
  })

  it('reopens once the client has given up', () => {
    ensureWsConnected()
    clients[0].connCb('disconnected')
    ensureWsConnected()
    expect(clients).toHaveLength(2)
    expect(get(wsConnectionState)).toBe('connecting')
  })

  it('ignores a late close from the client it replaced', () => {
    ensureWsConnected()
    reconnectWs()
    expect(clients[0].disconnected).toBe(true)
    clients[1].connCb('connected')
    clients[0].connCb('disconnected') // old socket's onclose lands after the new one opened
    expect(get(wsConnectionState)).toBe('connected')
  })

  it('delivers status pushes to subscribers until they unsubscribe', () => {
    const seen: string[] = []
    const off = onDaemonStatus((status) => seen.push(status))
    ensureWsConnected()
    clients[0].statusCb('running', '', false)
    off()
    clients[0].statusCb('stopped', '', false)
    expect(seen).toEqual(['running'])
  })
})
