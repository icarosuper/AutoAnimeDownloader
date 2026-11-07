import { writable } from 'svelte/store'
import type { WebSocketConnectionState } from '../websocket/client.js'

/**
 * The client only reports connected/reconnecting/disconnected; 'connecting' is the state before
 * its first answer. Starting at 'disconnected' painted the dot red on every page load.
 */
export type WsIndicatorState = WebSocketConnectionState | 'connecting'

/** Written only by lib/websocket/shared.ts — the one connection the app has. */
export const wsConnectionState = writable<WsIndicatorState>('connecting')
