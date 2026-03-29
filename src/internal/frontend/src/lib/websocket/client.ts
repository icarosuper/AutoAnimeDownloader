export interface WebSocketMessage {
  type: string
  data: {
    status: string
    last_check: string
    has_error: boolean
  }
}

export type WebSocketStatusCallback = (status: string, lastCheck: string, hasError: boolean) => void
export type WebSocketConnectionState = 'connected' | 'reconnecting' | 'disconnected'
export type WebSocketConnectionCallback = (state: WebSocketConnectionState) => void

export class WebSocketClient {
  private ws: WebSocket | null = null
  private url: string
  private reconnectInterval: number = 3000
  private maxReconnectAttempts: number = 10
  private reconnectAttempts: number = 0
  private shouldReconnect: boolean = true
  private statusCallback: WebSocketStatusCallback | null = null
  private connectionCallback: WebSocketConnectionCallback | null = null

  constructor(baseUrl: string = '') {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHost = baseUrl || window.location.host
    this.url = `${wsProtocol}//${wsHost}/api/v1/ws`
  }

  connect(callback: WebSocketStatusCallback, connectionCallback?: WebSocketConnectionCallback): void {
    this.statusCallback = callback
    this.connectionCallback = connectionCallback || null
    this.shouldReconnect = true
    this.reconnectAttempts = 0
    this.doConnect()
  }

  private doConnect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      return
    }

    try {
      this.ws = new WebSocket(this.url)

      this.ws.onopen = () => {
        console.log('WebSocket connected')
        this.reconnectAttempts = 0
        this.connectionCallback?.('connected')
      }

      this.ws.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data)
          if (message.type === 'status_update' && message.data && this.statusCallback) {
            this.statusCallback(
              message.data.status,
              message.data.last_check,
              message.data.has_error
            )
          }
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err)
        }
      }

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error)
      }

      this.ws.onclose = () => {
        console.log('WebSocket disconnected')
        this.ws = null

        if (this.shouldReconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
          this.reconnectAttempts++
          console.log(`Reconnecting WebSocket (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})...`)
          this.connectionCallback?.('reconnecting')
          setTimeout(() => this.doConnect(), this.reconnectInterval)
        } else {
          this.connectionCallback?.('disconnected')
          if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max WebSocket reconnection attempts reached')
          }
        }
      }
    } catch (err) {
      console.error('Failed to create WebSocket connection:', err)
      if (this.shouldReconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++
        this.connectionCallback?.('reconnecting')
        setTimeout(() => this.doConnect(), this.reconnectInterval)
      } else {
        this.connectionCallback?.('disconnected')
      }
    }
  }

  disconnect(): void {
    this.shouldReconnect = false
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }
}
