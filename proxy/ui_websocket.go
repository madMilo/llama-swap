package proxy

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local UI
	},
}

// WSMessage represents a WebSocket message sent to clients
type WSMessage struct {
	Channel  string `json:"channel"`  // Channel name (e.g., "logs.proxy", "chat.session123")
	Target   string `json:"target"`   // CSS selector for target element
	Action   string `json:"action"`   // Action: "append", "replace", "prepend"
	Content  string `json:"content"`  // HTML content to inject
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Optional metadata
}

// WSHub manages WebSocket connections and message broadcasting
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan WSMessage
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// WSClient represents a WebSocket client connection
type WSClient struct {
	hub           *WSHub
	conn          *websocket.Conn
	send          chan WSMessage
	subscriptions map[string]bool
	logCancels    map[string]func() // Cancel functions for log subscriptions
	mu            sync.RWMutex
	pm            *ProxyManager
}

// NewWSHub creates a new WebSocket hub
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan WSMessage, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the WebSocket hub
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				// Only send to clients subscribed to this channel
				client.mu.RLock()
				subscribed := client.subscriptions[message.Channel]
				client.mu.RUnlock()

				if subscribed {
					select {
					case client.send <- message:
					default:
						// Client buffer full, disconnect
						h.mu.RUnlock()
						h.unregister <- client
						h.mu.RLock()
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all subscribed clients
func (h *WSHub) Broadcast(message WSMessage) {
	select {
	case h.broadcast <- message:
	default:
		log.Printf("WebSocket broadcast channel full, dropping message for channel: %s", message.Channel)
	}
}

// HandleWebSocket handles WebSocket connections
func (pm *ProxyManager) HandleWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &WSClient{
		hub:           pm.wsHub,
		conn:          conn,
		send:          make(chan WSMessage, 256),
		subscriptions: make(map[string]bool),
		logCancels:    make(map[string]func()),
		pm:            pm,
	}

	client.hub.register <- client

	// Start goroutines
	go client.writePump()
	go client.readPump()
}

// readPump reads messages from the WebSocket connection
func (c *WSClient) readPump() {
	defer func() {
		// Cancel all log subscriptions
		c.mu.Lock()
		for _, cancel := range c.logCancels {
			cancel()
		}
		c.mu.Unlock()

		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		// Parse subscription message
		var sub struct {
			Action  string `json:"action"`  // "subscribe" or "unsubscribe"
			Channel string `json:"channel"` // Channel name
		}

		if err := json.Unmarshal(message, &sub); err != nil {
			log.Printf("WebSocket message parse error: %v", err)
			continue
		}

		c.mu.Lock()
		if sub.Action == "subscribe" {
			c.subscriptions[sub.Channel] = true
			c.handleChannelSubscribe(sub.Channel)
		} else if sub.Action == "unsubscribe" {
			delete(c.subscriptions, sub.Channel)
			c.handleChannelUnsubscribe(sub.Channel)
		}
		c.mu.Unlock()
	}
}

// writePump writes messages to the WebSocket connection
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send message as JSON
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleChannelSubscribe handles subscription to special channels
func (c *WSClient) handleChannelSubscribe(channel string) {
	switch channel {
	case "logs.proxy":
		if _, exists := c.logCancels[channel]; !exists {
			cancel := c.pm.proxyLogger.OnLogData(func(data []byte) {
				c.sendLogLine(channel, ".pre-block", string(data))
			})
			c.logCancels[channel] = cancel
		}
	case "logs.upstream":
		if _, exists := c.logCancels[channel]; !exists {
			cancel := c.pm.upstreamLogger.OnLogData(func(data []byte) {
				c.sendLogLine(channel, ".pre-block", string(data))
			})
			c.logCancels[channel] = cancel
		}
	}
}

// handleChannelUnsubscribe handles unsubscription from special channels
func (c *WSClient) handleChannelUnsubscribe(channel string) {
	if cancel, exists := c.logCancels[channel]; exists {
		cancel()
		delete(c.logCancels, channel)
	}
}

// sendLogLine sends a log line to the client
func (c *WSClient) sendLogLine(channel, target, content string) {
	select {
	case c.send <- WSMessage{
		Channel: channel,
		Target:  target,
		Action:  "append",
		Content: content,
	}:
	default:
		// Buffer full, drop message
	}
}
