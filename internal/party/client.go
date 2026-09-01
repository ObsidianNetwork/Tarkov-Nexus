package party

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tarkov-screenshot-analyzer/internal/logger"
)

// Client represents a party client that connects to the party server
type Client struct {
	logger       logger.Logger
	serverURL    string
	conn         *websocket.Conn
	partyCode    string
	remoteID     string
	displayName  string
	connected    bool
	mu           sync.RWMutex
	eventHandler EventHandler
	ctx          context.Context    // Context for cancellation
	cancel       context.CancelFunc // Cancel function
	reconnecting bool
	joinResponse chan error // Channel for synchronous join response
}

// EventHandler handles events from the party server
type EventHandler interface {
	OnWelcome(msg *WelcomeMessage)
	OnMemberJoined(msg *MemberJoinedMessage)
	OnMemberLeft(msg *MemberLeftMessage)
	OnPositionUpdate(msg *PositionUpdateMessage)
	OnError(msg *ErrorMessage)
	OnDisconnected()
}

// NewClient creates a new party client
func NewClient(serverURL string, log logger.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		logger:    log,
		serverURL: serverURL,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetEventHandler sets the event handler for the client
func (c *Client) SetEventHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

// Connect connects to the party server
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		c.logger.Warning("Already connected to party server")
		return fmt.Errorf("already connected")
	}

	u, err := url.Parse(c.serverURL)
	if err != nil {
		c.logger.Error(fmt.Sprintf("❌ Invalid party server URL '%s': %v", c.serverURL, err))
		return fmt.Errorf("invalid server URL: %w", err)
	}

	c.logger.Info(fmt.Sprintf("📡 Connecting to party server: %s", c.serverURL))

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		c.logger.Error(fmt.Sprintf("❌ Failed to connect to party server at %s: %v", c.serverURL, err))
		c.logger.Error("💡 Make sure the server URL is correct and the host is reachable on your network")
		return fmt.Errorf("failed to connect to party server: %w", err)
	}

	c.conn = conn
	c.connected = true
	c.logger.Info(fmt.Sprintf("✅ Connected to party server at %s", c.serverURL))

	// Start message handler
	go c.handleMessages()

	return nil
}

// Disconnect disconnects from the party server
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.logger.Info("Disconnecting from party server...")

	// Send leave message
	if c.partyCode != "" {
		leaveMsg := LeaveMessage{
			RemoteID: c.remoteID,
		}
		c.sendMessage(NewMessage(MessageTypeLeave, leaveMsg))
	}

	// Cancel context to stop all goroutines
	c.cancel()

	// Close connection
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.connected = false
	c.partyCode = ""

	// Recreate context for potential reconnection
	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.logger.Info("Disconnected from party server")

	// Notify event handler
	if c.eventHandler != nil {
		c.eventHandler.OnDisconnected()
	}

	return nil
}

// JoinParty joins a party with the given code and waits for server response
func (c *Client) JoinParty(partyCode, remoteID, displayName string) error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		c.logger.Error("❌ Cannot join party: Not connected to party server")
		return fmt.Errorf("not connected to party server")
	}

	// Create response channel for this join attempt
	c.joinResponse = make(chan error, 1)
	serverURL := c.serverURL
	c.mu.Unlock()

	// Store party info (but don't mark as joined yet)
	c.mu.Lock()
	c.remoteID = remoteID
	c.displayName = displayName
	c.mu.Unlock()

	c.logger.Info(fmt.Sprintf("🎮 Attempting to join party: Code=%s, DisplayName='%s', RemoteID=%s, Server=%s",
		partyCode, displayName, remoteID, serverURL))

	// Send join message
	joinMsg := JoinMessage{
		PartyCode:   partyCode,
		RemoteID:    remoteID,
		DisplayName: displayName,
	}

	if err := c.sendMessage(NewMessage(MessageTypeJoin, joinMsg)); err != nil {
		c.logger.Error(fmt.Sprintf("❌ Failed to send join message: %v", err))
		c.mu.Lock()
		c.joinResponse = nil
		c.mu.Unlock()
		return fmt.Errorf("failed to send join request: %w", err)
	}

	c.logger.Info("📤 Join request sent, waiting for server response...")

	// Wait for response with 10-second timeout
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	select {
	case err := <-c.joinResponse:
		if err != nil {
			c.logger.Error(fmt.Sprintf("❌ Join rejected by server: %v", err))
			return err
		}

		// Success! Mark as joined
		c.mu.Lock()
		c.partyCode = partyCode
		c.mu.Unlock()

		c.logger.Info(fmt.Sprintf("✅ Successfully joined party %s", partyCode))
		return nil

	case <-timeout.C:
		c.mu.Lock()
		c.joinResponse = nil
		c.mu.Unlock()

		c.logger.Error("❌ Join request timed out after 10 seconds - server did not respond")
		return fmt.Errorf("join request timed out - no response from server")
	}
}

// SendPosition sends a position update to the party
func (c *Client) SendPosition(mapName string, position *Position) error {
	c.mu.RLock()
	if !c.connected || c.partyCode == "" {
		c.mu.RUnlock()
		return fmt.Errorf("not connected to a party")
	}
	remoteID := c.remoteID
	c.mu.RUnlock()

	posMsg := PositionMessage{
		RemoteID: remoteID,
		Map:      mapName,
		Position: position,
	}

	return c.sendMessage(NewMessage(MessageTypePosition, posMsg))
}

// LeaveParty leaves the current party
func (c *Client) LeaveParty() error {
	c.mu.Lock()
	if !c.connected || c.partyCode == "" {
		c.mu.Unlock()
		return fmt.Errorf("not in a party")
	}

	remoteID := c.remoteID
	c.partyCode = ""
	c.mu.Unlock()

	c.logger.Info("Leaving party...")

	leaveMsg := LeaveMessage{
		RemoteID: remoteID,
	}

	return c.sendMessage(NewMessage(MessageTypeLeave, leaveMsg))
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// IsInParty returns whether the client is in a party
func (c *Client) IsInParty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.partyCode != ""
}

// GetPartyCode returns the current party code
func (c *Client) GetPartyCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.partyCode
}

// sendMessage sends a message to the server
func (c *Client) sendMessage(msg *Message) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(msg)
}

// handleMessages handles incoming messages from the server
func (c *Client) handleMessages() {
	// Capture context at start to avoid race conditions
	c.mu.RLock()
	ctx := c.ctx
	c.mu.RUnlock()

	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		if c.eventHandler != nil {
			c.eventHandler.OnDisconnected()
		}
	}()

	// Set up ping/pong
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn != nil {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		}
		return nil
	})

	// Start ping routine with captured context
	pingTicker := time.NewTicker(30 * time.Second)
	go func() {
		defer pingTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-pingTicker.C:
				c.mu.RLock()
				conn := c.conn
				c.mu.RUnlock()

				if conn == nil {
					return
				}

				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			c.mu.RLock()
			conn := c.conn
			c.mu.RUnlock()

			if conn == nil {
				return
			}

			_, messageBytes, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error(fmt.Sprintf("WebSocket error: %v", err))
				}
				return
			}

			var msg Message
			if err := json.Unmarshal(messageBytes, &msg); err != nil {
				c.logger.Error(fmt.Sprintf("Failed to parse message: %v", err))
				continue
			}

			c.handleMessage(&msg)
		}
	}
}

// handleMessage handles a single message from the server
func (c *Client) handleMessage(msg *Message) {
	dataBytes, _ := json.Marshal(msg.Data)

	switch msg.Type {
	case MessageTypeWelcome:
		var welcomeMsg WelcomeMessage
		if err := json.Unmarshal(dataBytes, &welcomeMsg); err == nil {
			c.logger.Info(fmt.Sprintf("✅ Received welcome message for party: %s", welcomeMsg.PartyCode))

			// Signal successful join if we're waiting for response
			c.mu.Lock()
			if c.joinResponse != nil {
				c.joinResponse <- nil // Success!
				c.joinResponse = nil
			}
			c.mu.Unlock()

			// Notify event handler
			if c.eventHandler != nil {
				c.eventHandler.OnWelcome(&welcomeMsg)
			}
		}

	case MessageTypeMemberJoined:
		var memberJoinedMsg MemberJoinedMessage
		if err := json.Unmarshal(dataBytes, &memberJoinedMsg); err == nil {
			c.logger.Info(fmt.Sprintf("Member joined: %s", memberJoinedMsg.Member.DisplayName))
			if c.eventHandler != nil {
				c.eventHandler.OnMemberJoined(&memberJoinedMsg)
			}
		}

	case MessageTypeMemberLeft:
		var memberLeftMsg MemberLeftMessage
		if err := json.Unmarshal(dataBytes, &memberLeftMsg); err == nil {
			c.logger.Info(fmt.Sprintf("Member left: %s", memberLeftMsg.RemoteID))
			if c.eventHandler != nil {
				c.eventHandler.OnMemberLeft(&memberLeftMsg)
			}
		}

	case MessageTypePositionUpdate:
		var posUpdateMsg PositionUpdateMessage
		if err := json.Unmarshal(dataBytes, &posUpdateMsg); err == nil {
			if c.eventHandler != nil {
				c.eventHandler.OnPositionUpdate(&posUpdateMsg)
			}
		}

	case MessageTypeError:
		var errorMsg ErrorMessage
		if err := json.Unmarshal(dataBytes, &errorMsg); err == nil {
			c.logger.Error(fmt.Sprintf("❌ Server error: [%s] %s", errorMsg.Code, errorMsg.Message))

			// If we're waiting for join response, this is a join failure
			c.mu.Lock()
			if c.joinResponse != nil {
				c.joinResponse <- fmt.Errorf("%s", errorMsg.Message)
				c.joinResponse = nil
			}
			c.mu.Unlock()

			// Notify event handler
			if c.eventHandler != nil {
				c.eventHandler.OnError(&errorMsg)
			}
		}
	}
}
