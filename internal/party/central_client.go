package party

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tarkov-screenshot-analyzer/internal/logger"
)

// CentralClient connects to the centralized party server
type CentralClient struct {
	logger       logger.Logger
	serverURL    string
	conn         *websocket.Conn
	clientID     string
	remoteID     string // tarkov.dev Remote ID for map markers
	displayName  string
	partyCode    string
	connected    bool
	registered   bool
	mu           sync.RWMutex
	eventHandler CentralEventHandler
	stopChan     chan struct{}
	joinResponse chan error
}

// CentralEventHandler handles events from the centralized party server
type CentralEventHandler interface {
	OnRegistered(clientID string)
	OnPartyCreated(partyCode string)
	OnPartyJoined(partyCode string, members []CentralMemberInfo)
	OnPartyLeft()
	OnMemberJoined(clientID, remoteID, displayName string)
	OnMemberLeft(clientID string)
	OnPositionUpdate(clientID, remoteID, displayName, mapName string, pos *Position)
	OnFriendsList(friends []FriendStatus)
	OnFriendOnline(clientID, displayName string)
	OnFriendOffline(clientID string)
	OnFriendAdded(clientID, displayName string)
	OnFriendRemoved(clientID string)
	OnFriendRequest(fromClientID, displayName string)
	OnFriendRequestSent(toClientID, displayName string)
	OnFriendRequestAccepted(clientID, displayName string)
	OnFriendRequestDeclined(clientID, displayName string)
	OnFriendRequestCancelled(fromClientID string)
	OnFriendRequestsList(incoming, outgoing []FriendRequestInfo)
	OnPartyInvite(fromClientID, fromName, partyCode string)
	OnInviteAccepted(clientID, displayName string)
	OnInviteDeclined(clientID, displayName string)
	OnInviteCancelled(fromClientID, partyCode string)
	OnError(code, message string)
	OnDisconnected()
}

// FriendRequestInfo represents a friend request
type FriendRequestInfo struct {
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
	SentAt      string `json:"sentAt"`
}

// CentralMemberInfo represents a party member
type CentralMemberInfo struct {
	ClientID    string    `json:"clientId"`
	RemoteID    string    `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string    `json:"displayName"`
	CurrentMap  string    `json:"currentMap,omitempty"`
	Position    *Position `json:"position,omitempty"`
	IsHost      bool      `json:"isHost"`
}

// FriendStatus represents a friend with online status
type FriendStatus struct {
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
	Online      bool   `json:"online"`
	InParty     bool   `json:"inParty"`
	PartyCode   string `json:"partyCode,omitempty"`
}

// Message types
const (
	// Client -> Server
	msgRegister              = "register"
	msgCreateParty           = "create_party"
	msgJoinParty             = "join_party"
	msgLeaveParty            = "leave_party"
	msgPosition              = "position"
	msgAddFriend             = "add_friend" // Now sends a friend request
	msgAcceptFriendRequest   = "accept_friend_request"
	msgDeclineFriendRequest  = "decline_friend_request"
	msgCancelFriendRequest   = "cancel_friend_request"
	msgGetPendingFriendReqs  = "get_pending_friend_requests"
	msgRemoveFriend          = "remove_friend"
	msgGetFriends            = "get_friends"
	msgInviteFriend          = "invite_friend"
	msgAcceptInvite          = "accept_invite"
	msgDeclineInvite         = "decline_invite"
	msgCancelInvite          = "cancel_invite"
	msgPing                  = "ping"

	// Server -> Client
	msgRegistered              = "registered"
	msgPartyCreated            = "party_created"
	msgPartyJoined             = "party_joined"
	msgPartyLeft               = "party_left"
	msgMemberJoined            = "member_joined"
	msgMemberLeft              = "member_left"
	msgPositionUpdate          = "position_update"
	msgFriendsList             = "friends_list"
	msgFriendOnline            = "friend_online"
	msgFriendOffline           = "friend_offline"
	msgFriendAdded             = "friend_added"
	msgFriendRemoved           = "friend_removed"
	msgFriendRequest           = "friend_request"
	msgFriendRequestSent       = "friend_request_sent"
	msgFriendRequestAccepted   = "friend_request_accepted"
	msgFriendRequestDeclined   = "friend_request_declined"
	msgFriendRequestCancelled  = "friend_request_cancelled"
	msgFriendRequestsList      = "friend_requests_list"
	msgPartyInvite             = "party_invite"
	msgInviteAccepted          = "invite_accepted"
	msgInviteDeclined          = "invite_declined"
	msgInviteCancelled         = "invite_cancelled"
	msgError                   = "error"
	msgPong                    = "pong"
)

// NewCentralClient creates a new centralized party client
func NewCentralClient(serverURL string, log logger.Logger) *CentralClient {
	return &CentralClient{
		logger:    log,
		serverURL: serverURL,
		stopChan:  make(chan struct{}),
	}
}

// SetEventHandler sets the event handler
func (c *CentralClient) SetEventHandler(handler CentralEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

// Connect connects to the centralized party server and registers
func (c *CentralClient) Connect(clientID, remoteID, displayName string) error {
	c.mu.Lock()
	if c.connected {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}

	c.clientID = clientID
	c.remoteID = remoteID
	c.displayName = displayName
	c.mu.Unlock()

	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Start message handler
	go c.handleMessages()

	// Register with server - include remoteId for tarkov.dev map markers
	if err := c.sendJSON(map[string]string{
		"type":        msgRegister,
		"clientId":    clientID,
		"remoteId":    remoteID,
		"displayName": displayName,
	}); err != nil {
		c.Disconnect()
		return fmt.Errorf("failed to register: %w", err)
	}

	return nil
}

// Disconnect disconnects from the server
func (c *CentralClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	close(c.stopChan)
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.connected = false
	c.registered = false
	c.partyCode = ""
	c.stopChan = make(chan struct{})

	if c.eventHandler != nil {
		go c.eventHandler.OnDisconnected()
	}

	return nil
}

// CreateParty creates a new party
func (c *CentralClient) CreateParty() error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	if c.partyCode != "" {
		c.mu.RUnlock()
		return fmt.Errorf("already in a party")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{"type": msgCreateParty})
}

// JoinParty joins an existing party
func (c *CentralClient) JoinParty(partyCode string) error {
	c.mu.Lock()
	if !c.connected || !c.registered {
		c.mu.Unlock()
		return fmt.Errorf("not connected or registered")
	}
	if c.partyCode != "" {
		c.mu.Unlock()
		return fmt.Errorf("already in a party")
	}

	c.joinResponse = make(chan error, 1)
	c.mu.Unlock()

	if err := c.sendJSON(map[string]string{
		"type":      msgJoinParty,
		"partyCode": partyCode,
	}); err != nil {
		c.mu.Lock()
		c.joinResponse = nil
		c.mu.Unlock()
		return err
	}

	// Wait for response with timeout
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	select {
	case err := <-c.joinResponse:
		return err
	case <-timeout.C:
		c.mu.Lock()
		c.joinResponse = nil
		c.mu.Unlock()
		return fmt.Errorf("join request timed out")
	}
}

// LeaveParty leaves the current party
func (c *CentralClient) LeaveParty() error {
	c.mu.RLock()
	if !c.connected || c.partyCode == "" {
		c.mu.RUnlock()
		return fmt.Errorf("not in a party")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{"type": msgLeaveParty})
}

// SendPosition sends position update to party members
func (c *CentralClient) SendPosition(mapName string, pos *Position) error {
	c.mu.RLock()
	if !c.connected || c.partyCode == "" {
		c.mu.RUnlock()
		return nil // Silently ignore if not in party
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]interface{}{
		"type":     msgPosition,
		"map":      mapName,
		"x":        pos.X,
		"y":        pos.Y,
		"z":        pos.Z,
		"rotation": pos.Rotation,
	})
}

// GetFriends requests the friends list
func (c *CentralClient) GetFriends() error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{"type": msgGetFriends})
}

// AddFriend sends a friend request (renamed from direct add)
func (c *CentralClient) AddFriend(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgAddFriend,
		"targetClientId": targetClientID,
	})
}

// AcceptFriendRequest accepts a friend request
func (c *CentralClient) AcceptFriendRequest(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgAcceptFriendRequest,
		"targetClientId": targetClientID,
	})
}

// DeclineFriendRequest declines a friend request
func (c *CentralClient) DeclineFriendRequest(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgDeclineFriendRequest,
		"targetClientId": targetClientID,
	})
}

// CancelFriendRequest cancels a sent friend request
func (c *CentralClient) CancelFriendRequest(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgCancelFriendRequest,
		"targetClientId": targetClientID,
	})
}

// GetPendingFriendRequests gets all pending friend requests
func (c *CentralClient) GetPendingFriendRequests() error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{"type": msgGetPendingFriendReqs})
}

// RemoveFriend removes a friend
func (c *CentralClient) RemoveFriend(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || !c.registered {
		c.mu.RUnlock()
		return fmt.Errorf("not connected or registered")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgRemoveFriend,
		"targetClientId": targetClientID,
	})
}

// InviteFriend invites a friend to current party
func (c *CentralClient) InviteFriend(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || c.partyCode == "" {
		c.mu.RUnlock()
		return fmt.Errorf("not in a party")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgInviteFriend,
		"targetClientId": targetClientID,
	})
}

// AcceptInvite accepts a party invite
func (c *CentralClient) AcceptInvite(partyCode string) error {
	return c.sendJSON(map[string]string{
		"type":      msgAcceptInvite,
		"partyCode": partyCode,
	})
}

// DeclineInvite declines a party invite
func (c *CentralClient) DeclineInvite(partyCode string) error {
	return c.sendJSON(map[string]string{
		"type":      msgDeclineInvite,
		"partyCode": partyCode,
	})
}

// CancelInvite cancels a sent party invite
func (c *CentralClient) CancelInvite(targetClientID string) error {
	c.mu.RLock()
	if !c.connected || c.partyCode == "" {
		c.mu.RUnlock()
		return fmt.Errorf("not in a party")
	}
	c.mu.RUnlock()

	return c.sendJSON(map[string]string{
		"type":           msgCancelInvite,
		"targetClientId": targetClientID,
	})
}

// IsConnected returns connection status
func (c *CentralClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.registered
}

// IsInParty returns party status
func (c *CentralClient) IsInParty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.partyCode != ""
}

// GetPartyCode returns current party code
func (c *CentralClient) GetPartyCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.partyCode
}

// GetClientID returns the client ID
func (c *CentralClient) GetClientID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID
}

func (c *CentralClient) sendJSON(v interface{}) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	return conn.WriteJSON(v)
}

func (c *CentralClient) handleMessages() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.registered = false
		c.mu.Unlock()

		if c.eventHandler != nil {
			c.eventHandler.OnDisconnected()
		}
	}()

	// Set up ping/pong
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Ping routine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.mu.RLock()
				conn := c.conn
				c.mu.RUnlock()
				if conn == nil {
					return
				}
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-c.stopChan:
				return
			}
		}
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error(fmt.Sprintf("WebSocket error: %v", err))
				}
				return
			}

			c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			c.handleMessage(data)
		}
	}
}

func (c *CentralClient) handleMessage(data []byte) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		c.logger.Error(fmt.Sprintf("Failed to parse message: %v", err))
		return
	}

	switch base.Type {
	case msgRegistered:
		var msg struct {
			ClientID string `json:"clientId"`
		}
		json.Unmarshal(data, &msg)
		c.mu.Lock()
		c.registered = true
		c.mu.Unlock()
		if c.eventHandler != nil {
			c.eventHandler.OnRegistered(msg.ClientID)
		}

	case msgPartyCreated:
		var msg struct {
			PartyCode string `json:"partyCode"`
		}
		json.Unmarshal(data, &msg)
		c.mu.Lock()
		c.partyCode = msg.PartyCode
		c.mu.Unlock()
		if c.eventHandler != nil {
			c.eventHandler.OnPartyCreated(msg.PartyCode)
		}

	case msgPartyJoined:
		var msg struct {
			PartyCode string             `json:"partyCode"`
			Members   []CentralMemberInfo `json:"members"`
		}
		json.Unmarshal(data, &msg)
		c.mu.Lock()
		c.partyCode = msg.PartyCode
		if c.joinResponse != nil {
			c.joinResponse <- nil
			c.joinResponse = nil
		}
		c.mu.Unlock()
		if c.eventHandler != nil {
			c.eventHandler.OnPartyJoined(msg.PartyCode, msg.Members)
		}

	case msgPartyLeft:
		c.mu.Lock()
		c.partyCode = ""
		c.mu.Unlock()
		if c.eventHandler != nil {
			c.eventHandler.OnPartyLeft()
		}

	case msgMemberJoined:
		var msg struct {
			ClientID    string `json:"clientId"`
			RemoteID    string `json:"remoteId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnMemberJoined(msg.ClientID, msg.RemoteID, msg.DisplayName)
		}

	case msgMemberLeft:
		var msg struct {
			ClientID string `json:"clientId"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnMemberLeft(msg.ClientID)
		}

	case msgPositionUpdate:
		var msg struct {
			ClientID    string `json:"clientId"`
			RemoteID    string `json:"remoteId"`
			DisplayName string `json:"displayName"`
			Map         string `json:"map"`
			Position    struct {
				X        float64 `json:"x"`
				Y        float64 `json:"y"`
				Z        float64 `json:"z"`
				Rotation float64 `json:"rotation"`
			} `json:"position"`
		}
		json.Unmarshal(data, &msg)
		pos := &Position{
			X:        msg.Position.X,
			Y:        msg.Position.Y,
			Z:        msg.Position.Z,
			Rotation: msg.Position.Rotation,
		}
		if c.eventHandler != nil {
			c.eventHandler.OnPositionUpdate(msg.ClientID, msg.RemoteID, msg.DisplayName, msg.Map, pos)
		}

	case msgFriendsList:
		var msg struct {
			Friends []FriendStatus `json:"friends"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendsList(msg.Friends)
		}

	case msgFriendOnline:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendOnline(msg.ClientID, msg.DisplayName)
		}

	case msgFriendOffline:
		var msg struct {
			ClientID string `json:"clientId"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendOffline(msg.ClientID)
		}

	case msgFriendAdded:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendAdded(msg.ClientID, msg.DisplayName)
		}

	case msgFriendRemoved:
		var msg struct {
			ClientID string `json:"clientId"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRemoved(msg.ClientID)
		}

	case msgFriendRequest:
		var msg struct {
			FromClientID string `json:"fromClientId"`
			DisplayName  string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequest(msg.FromClientID, msg.DisplayName)
		}

	case msgFriendRequestSent:
		var msg struct {
			ToClientID  string `json:"toClientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequestSent(msg.ToClientID, msg.DisplayName)
		}

	case msgFriendRequestAccepted:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequestAccepted(msg.ClientID, msg.DisplayName)
		}

	case msgFriendRequestDeclined:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequestDeclined(msg.ClientID, msg.DisplayName)
		}

	case msgFriendRequestCancelled:
		var msg struct {
			FromClientID string `json:"fromClientId"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequestCancelled(msg.FromClientID)
		}

	case msgFriendRequestsList:
		var msg struct {
			Incoming []FriendRequestInfo `json:"incoming"`
			Outgoing []FriendRequestInfo `json:"outgoing"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnFriendRequestsList(msg.Incoming, msg.Outgoing)
		}

	case msgPartyInvite:
		var msg struct {
			FromClientID string `json:"fromClientId"`
			FromName     string `json:"fromDisplayName"`
			PartyCode    string `json:"partyCode"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnPartyInvite(msg.FromClientID, msg.FromName, msg.PartyCode)
		}

	case msgInviteAccepted:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnInviteAccepted(msg.ClientID, msg.DisplayName)
		}

	case msgInviteDeclined:
		var msg struct {
			ClientID    string `json:"clientId"`
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnInviteDeclined(msg.ClientID, msg.DisplayName)
		}

	case msgInviteCancelled:
		var msg struct {
			FromClientID string `json:"fromClientId"`
			PartyCode    string `json:"partyCode"`
		}
		json.Unmarshal(data, &msg)
		if c.eventHandler != nil {
			c.eventHandler.OnInviteCancelled(msg.FromClientID, msg.PartyCode)
		}

	case msgError:
		var msg struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(data, &msg)
		c.logger.Error(fmt.Sprintf("Server error: [%s] %s", msg.Code, msg.Message))

		// Handle join failure
		c.mu.Lock()
		if c.joinResponse != nil {
			c.joinResponse <- fmt.Errorf("%s", msg.Message)
			c.joinResponse = nil
		}
		c.mu.Unlock()

		if c.eventHandler != nil {
			c.eventHandler.OnError(msg.Code, msg.Message)
		}

	case msgPong:
		// Ignore pong messages
	}
}
