package websocket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"
	"tarkov-screenshot-analyzer/internal/updater"

	"github.com/gorilla/websocket"
)

// clientName is the identity this application presents to tarkov.dev on the
// WebSocket handshake. tarkov.dev requires third-party clients to identify
// themselves via the Origin header so their traffic can be attributed; an
// unidentified client is indistinguishable from abuse and gets blocked.
const clientName = "TarkovNexus"

// maxReconnectBackoff caps the exponential reconnect delay.
const maxReconnectBackoff = 5 * time.Minute

// knownMaps is the set of map names tarkov.dev will accept, taken from the
// normalizedName values in their maps.json (the-hideout/tarkov-dev,
// src/data/maps.json).
//
// Both values matter to them. A `map` command navigates the paired browser to
// /map/<value>, so an unknown name lands the user on a 404 on their site; and a
// playerPosition is only drawn when its map equals the map key being viewed,
// which for the interactive projection is this same string.
//
// This guard exists because two separate resolvers in this repo disagreed and
// each emitted a name tarkov.dev does not have — "labs" and "night-factory".
// Validating here means a resolver bug can no longer reach their servers.
var knownMaps = map[string]bool{
	"customs":           true,
	"factory":           true,
	"ground-zero":       true,
	"interchange":       true,
	"lighthouse":        true,
	"openworld":         true,
	"reserve":           true,
	"shoreline":         true,
	"streets-of-tarkov": true,
	"the-lab":           true,
	"the-labyrinth":     true,
	"woods":             true,
}

// checkMapName rejects anything tarkov.dev would not recognise. It fails
// closed: refusing to send is better than navigating someone else's site to a
// page that does not exist. If tarkov.dev adds a map and this list is stale,
// the warning names the map so it is obvious from the in-app log.
func (c *Client) checkMapName(mapName, action string) error {
	if mapName == "" {
		return fmt.Errorf("refusing to send %s with empty map name", action)
	}
	if !knownMaps[mapName] {
		c.logger.Warning(fmt.Sprintf(
			"Refusing to send %s: %q is not a tarkov.dev map. If tarkov.dev has added it, "+
				"update knownMaps in internal/websocket/client.go", action, mapName))
		return fmt.Errorf("refusing to send %s: %q is not a known tarkov.dev map", action, mapName)
	}
	return nil
}

// Client represents a WebSocket client for tarkov.dev
type Client struct {
	remoteID             string
	logger               logger.Logger
	conn                 *websocket.Conn
	connMutex            sync.RWMutex
	reconnectInterval    time.Duration
	maxReconnectAttempts int
	reconnectAttempts    int
	shouldReconnect      bool
	isConnecting         bool
	ctx                  context.Context
	cancel               context.CancelFunc
	eventHandlers        map[string][]func(interface{})
	handlerMutex         sync.RWMutex

	// dialURL overrides the tarkov.dev endpoint. Empty in production; set by
	// tests so they can exercise the real Connect() — and therefore the real
	// handshake headers — against a local server.
	dialURL string

	// Connection statistics
	lastPingTime        time.Time
	connectionStartTime time.Time
	totalReconnects     int
	statsMutex          sync.RWMutex
}

// Message represents a WebSocket message
type Message struct {
	SessionID string      `json:"sessionID"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data,omitempty"`
}

// MapChangeData represents map change command data
type MapChangeData struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// PositionUpdateData represents position update command data
type PositionUpdateData struct {
	Type     string   `json:"type"`
	Map      string   `json:"map"`
	Position Position `json:"position"`
	Rotation float64  `json:"rotation"`
}

// Position represents a 3D position
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// NewClient creates a new WebSocket client
func NewClient(remoteID string, reconnectInterval time.Duration, maxReconnectAttempts int, log logger.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		remoteID:             remoteID,
		logger:               log,
		reconnectInterval:    reconnectInterval,
		maxReconnectAttempts: maxReconnectAttempts,
		shouldReconnect:      true,
		ctx:                  ctx,
		cancel:               cancel,
		eventHandlers:        make(map[string][]func(interface{})),
	}
}

// Connect establishes a WebSocket connection to tarkov.dev
func (c *Client) Connect() error {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.isConnecting || c.isConnected() {
		return nil
	}

	c.isConnecting = true
	defer func() { c.isConnecting = false }()

	// Based on TarkovMonitor implementation - WebSocket URL format
	wsURL := fmt.Sprintf("wss://socket.tarkov.dev?sessionid=%s-tm", url.QueryEscape(c.remoteID))
	if c.dialURL != "" {
		wsURL = c.dialURL
	}

	c.logger.Info(fmt.Sprintf("🌐 Connecting to tarkov.dev remote control: %s", wsURL))

	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Identify ourselves to the socket server. Both headers are required for
	// tarkov.dev to attribute this traffic to Tarkov Nexus rather than to an
	// anonymous client.
	header := http.Header{}
	header.Set("Origin", clientName)
	header.Set("User-Agent", fmt.Sprintf("%s/%s", clientName, updater.Version))

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		// Surface the HTTP status when the handshake was rejected — a 403
		// means we are blocked, which is very different from a network error.
		if resp != nil {
			return fmt.Errorf("failed to connect to WebSocket (HTTP %s): %w", resp.Status, err)
		}
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	c.conn = conn
	c.reconnectAttempts = 0

	// Update connection statistics
	c.statsMutex.Lock()
	c.connectionStartTime = time.Now()
	c.statsMutex.Unlock()

	c.logger.Info("✅ Connected to tarkov.dev remote control")
	c.emit("connected", nil)

	// Start message handler
	go c.handleMessages()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *Client) Disconnect() {
	c.shouldReconnect = false
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.cancel()
}

// IsConnected returns true if the WebSocket connection is open
func (c *Client) IsConnected() bool {
	c.connMutex.RLock()
	defer c.connMutex.RUnlock()
	return c.isConnected()
}

// GetConnectionStats returns connection statistics
func (c *Client) GetConnectionStats() map[string]interface{} {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	stats := map[string]interface{}{
		"totalReconnects": c.totalReconnects,
	}

	if !c.lastPingTime.IsZero() {
		stats["lastPingTime"] = c.lastPingTime.Format(time.RFC3339)
		stats["lastPingAgo"] = time.Since(c.lastPingTime).String()
	}

	if !c.connectionStartTime.IsZero() {
		stats["connectionStartTime"] = c.connectionStartTime.Format(time.RFC3339)
		stats["connectionUptime"] = time.Since(c.connectionStartTime).String()
	}

	return stats
}

// isConnected checks if connection is active (must be called with lock held)
func (c *Client) isConnected() bool {
	return c.conn != nil
}

// SendMapChange sends a map navigation command
func (c *Client) SendMapChange(mapName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	if err := c.checkMapName(mapName, "map change"); err != nil {
		return err
	}

	message := Message{
		SessionID: c.remoteID,
		Type:      "command",
		Data: MapChangeData{
			Type:  "map",
			Value: mapName,
		},
	}

	c.connMutex.RLock()
	defer c.connMutex.RUnlock()

	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send map change: %w", err)
	}

	c.logger.Debug(fmt.Sprintf("🗺️  Sent map change: %s", mapName))
	return nil
}

// SendPositionUpdate sends a position update
func (c *Client) SendPositionUpdate(x, y, z, rotation float64, mapName string) error {
	if !c.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	if err := c.checkMapName(mapName, "position update"); err != nil {
		return err
	}

	message := Message{
		SessionID: c.remoteID,
		Type:      "command",
		Data: PositionUpdateData{
			Type: "playerPosition",
			Map:  mapName,
			Position: Position{
				X: x,
				Y: y,
				Z: z,
			},
			Rotation: rotation,
		},
	}

	c.connMutex.RLock()
	defer c.connMutex.RUnlock()

	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send position update: %w", err)
	}

	return nil
}

// NOTE: Party member positions are deliberately NOT sent to tarkov.dev.
// tarkov.dev's socket protocol only defines "command" (relayed to the paired
// browser session) and "pong". Earlier versions sent a bespoke type:"party"
// envelope, which the server did not understand and which produced the
// server-side errors that got this application blocked (TAR-2). Party
// positions are delivered to the local overlay server instead, which is what
// actually renders the markers.

// SendPong sends a pong message
func (c *Client) SendPong() error {
	if !c.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	// tarkov.dev's own client replies with a bare {"type":"pong"}. Attaching a
	// sessionID makes the server treat the heartbeat reply as a relay message
	// addressed to a session, which is not what a pong is.
	message := map[string]string{"type": "pong"}

	c.connMutex.RLock()
	defer c.connMutex.RUnlock()

	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send pong: %w", err)
	}

	return nil
}

// OnEvent registers an event handler
func (c *Client) OnEvent(event string, handler func(interface{})) {
	c.handlerMutex.Lock()
	defer c.handlerMutex.Unlock()

	c.eventHandlers[event] = append(c.eventHandlers[event], handler)
}

// emit triggers event handlers
func (c *Client) emit(event string, data interface{}) {
	c.handlerMutex.RLock()
	defer c.handlerMutex.RUnlock()

	if handlers, exists := c.eventHandlers[event]; exists {
		for _, handler := range handlers {
			go func(h func(interface{}), ev string) {
				defer func() {
					if r := recover(); r != nil {
						c.logger.Error(fmt.Sprintf("Panic in websocket event handler [%s]: %v", ev, r))
					}
				}()
				h(data)
			}(handler, event)
		}
	}
}

// handleMessages processes incoming WebSocket messages
func (c *Client) handleMessages() {
	defer func() {
		c.connMutex.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connMutex.Unlock()

		c.emit("disconnected", nil)

		if c.shouldReconnect {
			c.attemptReconnect()
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			c.connMutex.RLock()
			conn := c.conn
			c.connMutex.RUnlock()

			if conn == nil {
				return
			}

			var message map[string]interface{}
			err := conn.ReadJSON(&message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error(fmt.Sprintf("❌ WebSocket error: %v", err))
				}
				return
			}

			c.handleMessage(message)
		}
	}
}

// handleMessage processes a single WebSocket message
func (c *Client) handleMessage(message map[string]interface{}) {
	msgType, ok := message["type"].(string)
	if !ok {
		return
	}

	switch msgType {
	case "ping":
		// Record ping time for statistics
		c.statsMutex.Lock()
		c.lastPingTime = time.Now()
		c.statsMutex.Unlock()
		c.SendPong()
	case "map_request":
		c.emit("mapRequest", message["map"])
	case "position_request":
		c.emit("positionRequest", nil)
	default:
		// The server relays message types this client does not consume; that is
		// expected traffic, not an error. Return rather than falling through:
		// the generic "message" emit below is for types we have actually
		// handled, and claiming to ignore something while still dispatching it
		// is the kind of half-measure that hides bugs.
		c.logger.Debug(fmt.Sprintf("Ignoring unhandled message type: %s", msgType))
		return
	}

	c.emit("message", message)
}

// attemptReconnect tries to reconnect to the WebSocket
// Uses a loop instead of recursive goroutines to prevent goroutine leaks
func (c *Client) attemptReconnect() {
	for {
		// Check context cancellation
		select {
		case <-c.ctx.Done():
			c.logger.Info("Reconnection cancelled")
			return
		default:
		}

		if c.reconnectAttempts >= c.maxReconnectAttempts {
			c.logger.Error(fmt.Sprintf("❌ Max reconnection attempts reached (%d)", c.maxReconnectAttempts))
			c.emit("maxReconnectAttemptsReached", nil)
			return
		}

		if !c.shouldReconnect {
			return
		}

		c.reconnectAttempts++
		c.logger.Info(fmt.Sprintf("🔄 Attempting to reconnect (%d/%d)...", c.reconnectAttempts, c.maxReconnectAttempts))

		// Exponential backoff, capped. A fixed short interval means a client
		// that is being rejected reconnects relentlessly, which is itself
		// abusive traffic from the server's point of view.
		backoff := c.reconnectInterval << uint(c.reconnectAttempts-1)
		if backoff > maxReconnectBackoff || backoff <= 0 {
			backoff = maxReconnectBackoff
		}

		// Use a timer with context so we can cancel the wait
		select {
		case <-c.ctx.Done():
			c.logger.Info("Reconnection cancelled during wait")
			return
		case <-time.After(backoff):
		}

		if !c.shouldReconnect {
			return
		}

		if err := c.Connect(); err != nil {
			c.logger.Warning(fmt.Sprintf("⚠️  Reconnection failed: %v", err))
			// Continue to next iteration instead of spawning new goroutine
			continue
		}

		// Successfully reconnected
		return
	}
}
