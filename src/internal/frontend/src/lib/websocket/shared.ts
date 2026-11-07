import { get } from 'svelte/store'
import { WebSocketClient, type WebSocketStatusCallback } from './client.js'
import { wsConnectionState } from '../stores/wsState.js'

// One WebSocket for the whole app. The connection dot lives in the shell (NavRail/MoreMenu), on
// every screen, so the connection has to outlive any one screen: AppShell opens it on mount.
// Screens used to open their own and close it in onDestroy — leaving Status turned the dot red,
// and landing straight on #/config showed red with the daemon fine.

let client: WebSocketClient | null = null
const statusListeners = new Set<WebSocketStatusCallback>()

function open(): void {
  client?.disconnect()
  const c = new WebSocketClient()
  client = c
  wsConnectionState.set('connecting')
  c.connect(
    (status, lastCheck, hasError) => {
      for (const listener of statusListeners) listener(status, lastCheck, hasError)
    },
    // A replaced client still fires 'disconnected' from its own onclose, possibly after the new
    // one connected — only the current client may write the state.
    (state) => {
      if (client === c) wsConnectionState.set(state)
    },
  )
}

/**
 * Opens the connection unless one is already up or retrying. The client gives up after ~30s of
 * failed retries ('disconnected'); calling this from a screen's onMount is what revives it, as
 * navigating back to Status did when each screen owned its own socket.
 */
export function ensureWsConnected(): void {
  if (client && get(wsConnectionState) !== 'disconnected') return
  open()
}

/** Drops the current connection and starts over (the Downloads banner's "Reconnect"). */
export function reconnectWs(): void {
  open()
}

/** Subscribes to daemon status pushes. Returns the unsubscribe function, for onDestroy. */
export function onDaemonStatus(listener: WebSocketStatusCallback): () => void {
  statusListeners.add(listener)
  return () => {
    statusListeners.delete(listener)
  }
}

/** Tests only: forget the connection and the listeners between cases. */
export function resetSharedWs(): void {
  client?.disconnect()
  client = null
  statusListeners.clear()
  wsConnectionState.set('connecting')
}
