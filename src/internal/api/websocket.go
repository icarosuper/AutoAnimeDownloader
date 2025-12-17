package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/logger"
	"context"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// WebSocketManager manages WebSocket connections and broadcasts state updates
type WebSocketManager struct {
	mu          sync.RWMutex
	clients     map[*websocket.Conn]struct{}
	broadcast   chan StateUpdateMessage
	stateGetter func() (daemon.Status, time.Time, bool)
}

// StateUpdateMessage represents a WebSocket message for state updates
type StateUpdateMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// StatusUpdateData represents the data payload for status updates
type StatusUpdateData struct {
	Status    string    `json:"status"`
	LastCheck time.Time `json:"last_check"`
	HasError  bool      `json:"has_error"`
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager() *WebSocketManager {
	manager := &WebSocketManager{
		clients:   make(map[*websocket.Conn]struct{}),
		broadcast: make(chan StateUpdateMessage, 256),
	}

	// Start broadcast goroutine
	go manager.broadcastLoop()

	return manager
}

// NotifyStateChange implements daemon.StateNotifier interface
func (wsm *WebSocketManager) NotifyStateChange(status daemon.Status, lastCheck time.Time, hasError bool) {
	message := StateUpdateMessage{
		Type: "status_update",
		Data: StatusUpdateData{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
		},
	}

	// Non-blocking send
	select {
	case wsm.broadcast <- message:
	default:
		logger.Logger.Warn().Msg("WebSocket broadcast channel full, dropping message")
	}
}

// broadcastLoop continuously broadcasts messages to all connected clients
func (wsm *WebSocketManager) broadcastLoop() {
	for message := range wsm.broadcast {
		wsm.mu.RLock()
		clients := make([]*websocket.Conn, 0, len(wsm.clients))
		for conn := range wsm.clients {
			clients = append(clients, conn)
		}
		wsm.mu.RUnlock()

		// Broadcast to all clients
		for _, conn := range clients {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := wsjson.Write(ctx, conn, message)
			cancel()

			if err != nil {
				logger.Logger.Debug().Err(err).Msg("Failed to send WebSocket message, removing client")
				wsm.removeClient(conn)
			}
		}
	}
}

// addClient adds a new WebSocket client
func (wsm *WebSocketManager) addClient(conn *websocket.Conn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.clients[conn] = struct{}{}
	logger.Logger.Info().Int("clients", len(wsm.clients)).Msg("WebSocket client connected")
}

// removeClient removes a WebSocket client
func (wsm *WebSocketManager) removeClient(conn *websocket.Conn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	delete(wsm.clients, conn)
	conn.Close(websocket.StatusNormalClosure, "")
	logger.Logger.Info().Int("clients", len(wsm.clients)).Msg("WebSocket client disconnected")
}

// HandleWebSocket handles WebSocket connections
func (wsm *WebSocketManager) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade connection to WebSocket
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"}, // Allow all origins in development
	})
	if err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to upgrade connection to WebSocket")
		return
	}
	defer conn.Close(websocket.StatusInternalError, "Internal error")

	// Add client
	wsm.addClient(conn)
	defer wsm.removeClient(conn)

	// Send initial state
	ctx := r.Context()
	status, lastCheck, hasError := wsm.getCurrentState()
	initialMessage := StateUpdateMessage{
		Type: "status_update",
		Data: StatusUpdateData{
			Status:    string(status),
			LastCheck: lastCheck,
			HasError:  hasError,
		},
	}
	if err := wsjson.Write(ctx, conn, initialMessage); err != nil {
		logger.Logger.Error().Err(err).Msg("Failed to send initial state")
		return
	}

	// Set read limit
	conn.SetReadLimit(512)

	// Ping loop to keep connection alive
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := conn.Ping(ctx)
				cancel()
				if err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Read loop (for pong responses and potential future client messages)
	// Use a context with timeout for reading
	readCtx, readCancel := context.WithCancel(ctx)
	defer readCancel()
	
	go func() {
		for {
			select {
			case <-readCtx.Done():
				return
			default:
				_, _, err := conn.Read(readCtx)
				if err != nil {
					if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
						websocket.CloseStatus(err) == websocket.StatusGoingAway {
						logger.Logger.Debug().Msg("WebSocket client closed connection")
					} else {
						logger.Logger.Debug().Err(err).Msg("WebSocket read error")
					}
					readCancel()
					return
				}
			}
		}
	}()
	
	// Wait for context cancellation (client disconnect or server shutdown)
	<-readCtx.Done()
}

// getCurrentState gets the current state from the server
func (wsm *WebSocketManager) getCurrentState() (daemon.Status, time.Time, bool) {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()
	if wsm.stateGetter != nil {
		return wsm.stateGetter()
	}
	return daemon.StatusStopped, time.Time{}, false
}

// SetStateGetter sets a function to get the current state
func (wsm *WebSocketManager) SetStateGetter(getter func() (daemon.Status, time.Time, bool)) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.stateGetter = getter
}

// Close closes all WebSocket connections
func (wsm *WebSocketManager) Close() {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	close(wsm.broadcast)

	for conn := range wsm.clients {
		conn.Close(websocket.StatusGoingAway, "Server shutting down")
	}
	wsm.clients = make(map[*websocket.Conn]struct{})
}

