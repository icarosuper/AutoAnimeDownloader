import { writable } from 'svelte/store'
import type { WebSocketConnectionState } from '../websocket/client.js'

export const wsConnectionState = writable<WebSocketConnectionState>('disconnected')
