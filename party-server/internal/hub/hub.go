package hub

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tarkov-screenshot-analyzer/party-server/internal/models"
	"tarkov-screenshot-analyzer/party-server/internal/storage"
)

// NATO phonetic alphabet for party codes
var natoAlphabet = []string{
	"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO", "FOXTROT",
	"GOLF", "HOTEL", "INDIA", "JULIET", "KILO", "LIMA",
	"MIKE", "NOVEMBER", "OSCAR", "PAPA", "QUEBEC", "ROMEO",
	"SIERRA", "TANGO", "UNIFORM", "VICTOR", "WHISKEY", "XRAY",
	"YANKEE", "ZULU",
}

// Hub manages all WebSocket connections, parties, and friends
type Hub struct {
	clients     map[string]*models.Client // clientID -> Client
	parties     map[string]*models.Party  // partyCode -> Party
	invites     map[string][]*models.PartyInvite // toClientID -> invites
	storage     *storage.Storage
	mu          sync.RWMutex
	inviteMu    sync.RWMutex
	done        chan struct{} // closed by Close to stop the cleanup routine
	closeOnce   sync.Once
}

// NewHub creates a new Hub instance
func NewHub(store *storage.Storage) *Hub {
	h := &Hub{
		clients: make(map[string]*models.Client),
		parties: make(map[string]*models.Party),
		invites: make(map[string][]*models.PartyInvite),
		storage: store,
		done:    make(chan struct{}),
	}

	// Start cleanup routine
	go h.cleanupRoutine()

	return h
}

// cleanupRoutine periodically cleans up empty parties and expired invites
func (h *Hub) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupEmptyParties()
			h.cleanupExpiredInvites()
		case <-h.done:
			return
		}
	}
}

// Close stops the background cleanup routine. Safe to call multiple times,
// including concurrently. It does not close the underlying storage — the
// caller owns that.
func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.done) })
}

func (h *Hub) cleanupEmptyParties() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for code, party := range h.parties {
		if party.IsEmpty() {
			log.Printf("[Hub] Cleaning up empty party: %s", code)
			delete(h.parties, code)
		}
	}
}

func (h *Hub) cleanupExpiredInvites() {
	h.inviteMu.Lock()
	defer h.inviteMu.Unlock()

	now := time.Now()
	for clientID, invites := range h.invites {
		var valid []*models.PartyInvite
		for _, inv := range invites {
			if inv.ExpiresAt.After(now) {
				valid = append(valid, inv)
			}
		}
		if len(valid) > 0 {
			h.invites[clientID] = valid
		} else {
			delete(h.invites, clientID)
		}
	}
}

// HandleConnection handles a new WebSocket connection
func (h *Hub) HandleConnection(conn *websocket.Conn) {
	defer conn.Close()

	var client *models.Client

	// Set up ping/pong
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping routine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Hub] Read error: %v", err)
			}
			break
		}

		// Reset read deadline on message
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		msg, err := models.ParseMessage(message)
		if err != nil {
			h.sendError(conn, models.ErrCodeInvalidMessage, "Failed to parse message")
			continue
		}

		switch m := msg.(type) {
		case *models.RegisterMessage:
			client = h.handleRegister(conn, m)

		case *models.CreatePartyMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleCreateParty(client)

		case *models.JoinPartyMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleJoinParty(client, m)

		case *models.LeavePartyMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleLeaveParty(client)

		case *models.PositionMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handlePosition(client, m)

		case *models.AddFriendMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleAddFriend(client, m)

		case *models.AcceptFriendRequestMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleAcceptFriendRequest(client, m)

		case *models.DeclineFriendRequestMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleDeclineFriendRequest(client, m)

		case *models.CancelFriendRequestMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleCancelFriendRequest(client, m)

		case *models.GetPendingFriendRequestsMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleGetPendingFriendRequests(client)

		case *models.RemoveFriendMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleRemoveFriend(client, m)

		case *models.GetFriendsMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleGetFriends(client)

		case *models.InviteFriendMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleInviteFriend(client, m)

		case *models.AcceptInviteMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleAcceptInvite(client, m)

		case *models.DeclineInviteMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleDeclineInvite(client, m)

		case *models.CancelInviteMessage:
			if client == nil {
				h.sendError(conn, models.ErrCodeNotRegistered, "Must register first")
				continue
			}
			h.handleCancelInvite(client, m)

		case *models.BaseMessage:
			if m.Type == models.MsgTypePing {
				h.sendMessage(conn, &models.PongMessage{Type: models.MsgTypePong})
			}
		}
	}

	// Cleanup on disconnect
	if client != nil {
		h.handleDisconnect(client)
	}
}

// handleRegister processes a registration message
func (h *Hub) handleRegister(conn *websocket.Conn, msg *models.RegisterMessage) *models.Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if already connected
	if existing, ok := h.clients[msg.ClientID]; ok {
		// Close old connection
		existing.Conn.Close()
	}

	// Store in database
	if err := h.storage.RegisterClient(msg.ClientID, msg.DisplayName); err != nil {
		log.Printf("[Hub] Failed to store client: %v", err)
	}

	client := &models.Client{
		ID:          msg.ClientID,
		RemoteID:    msg.RemoteID, // tarkov.dev Remote ID for map markers
		DisplayName: msg.DisplayName,
		Conn:        conn,
		ConnectedAt: time.Now(),
	}

	h.clients[msg.ClientID] = client
	log.Printf("[Hub] Client registered: %s (%s)", msg.DisplayName, msg.ClientID[:8])

	// Send registration confirmation
	h.sendMessage(conn, &models.RegisteredMessage{
		Type:     models.MsgTypeRegistered,
		ClientID: msg.ClientID,
	})

	// Notify friends that this client is online
	go h.notifyFriendsOnline(client)

	return client
}

// handleCreateParty creates a new party
func (h *Hub) handleCreateParty(client *models.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if already in a party
	if client.GetParty() != "" {
		h.sendError(client.Conn, models.ErrCodeAlreadyInParty, "Already in a party")
		return
	}

	// Generate unique party code
	var code string
	for {
		code = h.generatePartyCode()
		if _, exists := h.parties[code]; !exists {
			break
		}
	}

	// Create party
	party := models.NewParty(code, client.ID)
	party.AddMember(client)
	h.parties[code] = party

	log.Printf("[Hub] Party created: %s by %s", code, client.DisplayName)

	h.sendMessage(client.Conn, &models.PartyCreatedMessage{
		Type:      models.MsgTypePartyCreated,
		PartyCode: code,
	})
}

// handleJoinParty joins an existing party
func (h *Hub) handleJoinParty(client *models.Client, msg *models.JoinPartyMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if already in a party
	if client.GetParty() != "" {
		h.sendError(client.Conn, models.ErrCodeAlreadyInParty, "Already in a party")
		return
	}

	// Normalize party code
	code := strings.ToUpper(strings.TrimSpace(msg.PartyCode))

	party, exists := h.parties[code]
	if !exists {
		h.sendError(client.Conn, models.ErrCodeInvalidParty, "Party not found")
		return
	}

	if !party.AddMember(client) {
		h.sendError(client.Conn, models.ErrCodePartyFull, "Party is full (max 5 members)")
		return
	}

	log.Printf("[Hub] %s joined party %s", client.DisplayName, code)

	// Build member list
	members := make([]models.MemberInfo, 0)
	for _, m := range party.GetMembers() {
		pos, mapName, _ := m.GetPosition()
		members = append(members, models.MemberInfo{
			ClientID:    m.ID,
			RemoteID:    m.RemoteID,
			DisplayName: m.DisplayName,
			CurrentMap:  mapName,
			Position:    pos,
			IsHost:      m.ID == party.GetHost(),
		})
	}

	// Send join confirmation
	h.sendMessage(client.Conn, &models.PartyJoinedMessage{
		Type:      models.MsgTypePartyJoined,
		PartyCode: code,
		Members:   members,
	})

	// Notify other members
	for _, m := range party.GetMembers() {
		if m.ID != client.ID {
			h.sendMessage(m.Conn, &models.MemberJoinedMessage{
				Type:        models.MsgTypeMemberJoined,
				ClientID:    client.ID,
				RemoteID:    client.RemoteID,
				DisplayName: client.DisplayName,
			})

			// Update last played with
			go func(memberID string) {
				h.storage.UpdateLastPlayedWith(client.ID, memberID)
			}(m.ID)
		}
	}
}

// handleLeaveParty leaves the current party
func (h *Hub) handleLeaveParty(client *models.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	code := client.GetParty()
	if code == "" {
		h.sendError(client.Conn, models.ErrCodeNotInParty, "Not in a party")
		return
	}

	party, exists := h.parties[code]
	if !exists {
		client.SetParty("")
		return
	}

	// Notify other members before leaving
	for _, m := range party.GetMembers() {
		if m.ID != client.ID {
			h.sendMessage(m.Conn, &models.MemberLeftMessage{
				Type:     models.MsgTypeMemberLeft,
				ClientID: client.ID,
			})
		}
	}

	party.RemoveMember(client.ID)
	log.Printf("[Hub] %s left party %s", client.DisplayName, code)

	// Send confirmation
	h.sendMessage(client.Conn, &models.PartyLeftMessage{
		Type: models.MsgTypePartyLeft,
	})

	// Clean up empty party
	if party.IsEmpty() {
		delete(h.parties, code)
		log.Printf("[Hub] Party %s deleted (empty)", code)
	}
}

// handlePosition broadcasts position to party members
func (h *Hub) handlePosition(client *models.Client, msg *models.PositionMessage) {
	code := client.GetParty()
	if code == "" {
		return // Silently ignore if not in party
	}

	// Update client position
	client.UpdatePosition(&models.Position{
		X:        msg.X,
		Y:        msg.Y,
		Z:        msg.Z,
		Rotation: msg.Rotation,
	}, msg.Map)

	h.mu.RLock()
	party, exists := h.parties[code]
	h.mu.RUnlock()

	if !exists {
		return
	}

	// Broadcast to other members
	update := &models.PositionUpdateMessage{
		Type:        models.MsgTypePositionUpdate,
		ClientID:    client.ID,
		RemoteID:    client.RemoteID,
		DisplayName: client.DisplayName,
		Map:         msg.Map,
		Position: models.Position{
			X:        msg.X,
			Y:        msg.Y,
			Z:        msg.Z,
			Rotation: msg.Rotation,
		},
	}

	for _, m := range party.GetMembers() {
		if m.ID != client.ID {
			h.sendMessage(m.Conn, update)
		}
	}
}

// handleAddFriend sends a friend request (renamed from direct add)
func (h *Hub) handleAddFriend(client *models.Client, msg *models.AddFriendMessage) {
	h.mu.RLock()
	target, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if !exists {
		h.sendError(client.Conn, models.ErrCodeFriendNotFound, "User not found or not online")
		return
	}

	// Check if already friends
	areFriends, err := h.storage.AreFriends(client.ID, msg.TargetClientID)
	if err != nil {
		log.Printf("[Hub] Error checking friends: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend status")
		return
	}

	if areFriends {
		h.sendError(client.Conn, models.ErrCodeAlreadyFriends, "Already friends with this user")
		return
	}

	// Check if a request already exists from us to them
	requestExists, err := h.storage.GetFriendRequest(client.ID, msg.TargetClientID)
	if err != nil {
		log.Printf("[Hub] Error checking friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend request")
		return
	}

	if requestExists {
		h.sendError(client.Conn, models.ErrCodeAlreadyFriends, "Friend request already sent")
		return
	}

	// Check if they already sent us a request - if so, auto-accept
	reverseRequestExists, err := h.storage.GetFriendRequest(msg.TargetClientID, client.ID)
	if err != nil {
		log.Printf("[Hub] Error checking reverse friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend request")
		return
	}

	if reverseRequestExists {
		// They already sent us a request, so accept it (mutual add)
		h.acceptFriendRequest(client, msg.TargetClientID, target)
		return
	}

	// Create friend request
	if err := h.storage.CreateFriendRequest(client.ID, msg.TargetClientID, client.DisplayName, target.DisplayName); err != nil {
		log.Printf("[Hub] Error creating friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to send friend request")
		return
	}

	log.Printf("[Hub] %s sent friend request to %s", client.DisplayName, target.DisplayName)

	// Notify the sender that request was sent
	h.sendMessage(client.Conn, &models.FriendRequestSentMessage{
		Type:        models.MsgTypeFriendRequestSent,
		ToClientID:  target.ID,
		DisplayName: target.DisplayName,
	})

	// Notify the target that they have a friend request
	h.sendMessage(target.Conn, &models.FriendRequestMessage{
		Type:         models.MsgTypeFriendRequest,
		FromClientID: client.ID,
		DisplayName:  client.DisplayName,
	})
}

// acceptFriendRequest is a helper to accept a friend request
func (h *Hub) acceptFriendRequest(client *models.Client, fromClientID string, fromClient *models.Client) {
	// Delete the request
	if err := h.storage.DeleteFriendRequest(fromClientID, client.ID); err != nil {
		log.Printf("[Hub] Error deleting friend request: %v", err)
	}

	// Get display names
	fromDisplayName := ""
	if fromClient != nil {
		fromDisplayName = fromClient.DisplayName
	} else {
		fromDisplayName, _, _ = h.storage.GetClient(fromClientID)
	}

	// Add friend relationship
	if err := h.storage.AddFriend(client.ID, fromClientID, fromDisplayName); err != nil {
		log.Printf("[Hub] Error adding friend: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to add friend")
		return
	}

	log.Printf("[Hub] %s and %s are now friends", client.DisplayName, fromDisplayName)

	// Notify the accepter
	h.sendMessage(client.Conn, &models.FriendAddedMessage{
		Type:        models.MsgTypeFriendAdded,
		ClientID:    fromClientID,
		DisplayName: fromDisplayName,
	})

	// Notify the requester (if online)
	if fromClient != nil {
		h.sendMessage(fromClient.Conn, &models.FriendRequestAcceptedMessage{
			Type:        models.MsgTypeFriendRequestAccepted,
			ClientID:    client.ID,
			DisplayName: client.DisplayName,
		})
		h.sendMessage(fromClient.Conn, &models.FriendAddedMessage{
			Type:        models.MsgTypeFriendAdded,
			ClientID:    client.ID,
			DisplayName: client.DisplayName,
		})
	}
}

// handleAcceptFriendRequest handles accepting a friend request
func (h *Hub) handleAcceptFriendRequest(client *models.Client, msg *models.AcceptFriendRequestMessage) {
	// Check if request exists
	requestExists, err := h.storage.GetFriendRequest(msg.TargetClientID, client.ID)
	if err != nil {
		log.Printf("[Hub] Error checking friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend request")
		return
	}

	if !requestExists {
		h.sendError(client.Conn, models.ErrCodeInviteNotFound, "Friend request not found")
		return
	}

	h.mu.RLock()
	fromClient, _ := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	h.acceptFriendRequest(client, msg.TargetClientID, fromClient)
}

// handleDeclineFriendRequest handles declining a friend request
func (h *Hub) handleDeclineFriendRequest(client *models.Client, msg *models.DeclineFriendRequestMessage) {
	// Check if request exists
	requestExists, err := h.storage.GetFriendRequest(msg.TargetClientID, client.ID)
	if err != nil {
		log.Printf("[Hub] Error checking friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend request")
		return
	}

	if !requestExists {
		h.sendError(client.Conn, models.ErrCodeInviteNotFound, "Friend request not found")
		return
	}

	// Delete the request
	if err := h.storage.DeleteFriendRequest(msg.TargetClientID, client.ID); err != nil {
		log.Printf("[Hub] Error deleting friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to decline friend request")
		return
	}

	log.Printf("[Hub] %s declined friend request from %s", client.DisplayName, msg.TargetClientID[:8])

	// Notify the requester (if online)
	h.mu.RLock()
	fromClient, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(fromClient.Conn, &models.FriendRequestDeclinedMessage{
			Type:        models.MsgTypeFriendRequestDeclined,
			ClientID:    client.ID,
			DisplayName: client.DisplayName,
		})
	}
}

// handleCancelFriendRequest handles cancelling a sent friend request
func (h *Hub) handleCancelFriendRequest(client *models.Client, msg *models.CancelFriendRequestMessage) {
	// Check if request exists
	requestExists, err := h.storage.GetFriendRequest(client.ID, msg.TargetClientID)
	if err != nil {
		log.Printf("[Hub] Error checking friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to check friend request")
		return
	}

	if !requestExists {
		h.sendError(client.Conn, models.ErrCodeInviteNotFound, "Friend request not found")
		return
	}

	// Delete the request
	if err := h.storage.DeleteFriendRequest(client.ID, msg.TargetClientID); err != nil {
		log.Printf("[Hub] Error deleting friend request: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to cancel friend request")
		return
	}

	log.Printf("[Hub] %s cancelled friend request to %s", client.DisplayName, msg.TargetClientID[:8])

	// Notify the target (if online)
	h.mu.RLock()
	toClient, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(toClient.Conn, &models.FriendRequestCancelledMessage{
			Type:         models.MsgTypeFriendRequestCancelled,
			FromClientID: client.ID,
		})
	}
}

// handleGetPendingFriendRequests returns all pending friend requests
func (h *Hub) handleGetPendingFriendRequests(client *models.Client) {
	incoming, err := h.storage.GetIncomingFriendRequests(client.ID)
	if err != nil {
		log.Printf("[Hub] Error getting incoming friend requests: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to get friend requests")
		return
	}

	outgoing, err := h.storage.GetOutgoingFriendRequests(client.ID)
	if err != nil {
		log.Printf("[Hub] Error getting outgoing friend requests: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to get friend requests")
		return
	}

	incomingList := make([]models.FriendRequestInfo, len(incoming))
	for i, r := range incoming {
		incomingList[i] = models.FriendRequestInfo{
			ClientID:    r.ClientID,
			DisplayName: r.DisplayName,
			SentAt:      r.SentAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	outgoingList := make([]models.FriendRequestInfo, len(outgoing))
	for i, r := range outgoing {
		outgoingList[i] = models.FriendRequestInfo{
			ClientID:    r.ClientID,
			DisplayName: r.DisplayName,
			SentAt:      r.SentAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	h.sendMessage(client.Conn, &models.FriendRequestsListMessage{
		Type:     models.MsgTypeFriendRequestsList,
		Incoming: incomingList,
		Outgoing: outgoingList,
	})
}

// handleRemoveFriend removes a friend
func (h *Hub) handleRemoveFriend(client *models.Client, msg *models.RemoveFriendMessage) {
	if err := h.storage.RemoveFriend(client.ID, msg.TargetClientID); err != nil {
		log.Printf("[Hub] Error removing friend: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to remove friend")
		return
	}

	log.Printf("[Hub] %s removed friend %s", client.DisplayName, msg.TargetClientID[:8])

	h.sendMessage(client.Conn, &models.FriendRemovedMessage{
		Type:     models.MsgTypeFriendRemoved,
		ClientID: msg.TargetClientID,
	})

	// Notify the other user if online
	h.mu.RLock()
	target, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(target.Conn, &models.FriendRemovedMessage{
			Type:     models.MsgTypeFriendRemoved,
			ClientID: client.ID,
		})
	}
}

// handleGetFriends sends the friends list
func (h *Hub) handleGetFriends(client *models.Client) {
	friends, err := h.storage.GetFriends(client.ID)
	if err != nil {
		log.Printf("[Hub] Error getting friends: %v", err)
		h.sendError(client.Conn, models.ErrCodeInternalError, "Failed to get friends list")
		return
	}

	h.mu.RLock()
	friendStatuses := make([]models.FriendStatus, len(friends))
	for i, f := range friends {
		status := models.FriendStatus{
			ClientID:    f.FriendID,
			DisplayName: f.FriendName,
			Online:      false,
			InParty:     false,
		}

		if onlineClient, exists := h.clients[f.FriendID]; exists {
			status.Online = true
			status.DisplayName = onlineClient.DisplayName // Use current display name
			if partyCode := onlineClient.GetParty(); partyCode != "" {
				status.InParty = true
				status.PartyCode = partyCode
			}
		}

		friendStatuses[i] = status
	}
	h.mu.RUnlock()

	h.sendMessage(client.Conn, &models.FriendsListMessage{
		Type:    models.MsgTypeFriendsList,
		Friends: friendStatuses,
	})
}

// handleInviteFriend sends a party invite to a friend
func (h *Hub) handleInviteFriend(client *models.Client, msg *models.InviteFriendMessage) {
	code := client.GetParty()
	if code == "" {
		h.sendError(client.Conn, models.ErrCodeNotInParty, "Must be in a party to invite")
		return
	}

	h.mu.RLock()
	target, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if !exists {
		h.sendError(client.Conn, models.ErrCodeFriendNotFound, "User not online")
		return
	}

	// Create invite
	invite := &models.PartyInvite{
		FromClientID: client.ID,
		FromName:     client.DisplayName,
		ToClientID:   msg.TargetClientID,
		PartyCode:    code,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}

	h.inviteMu.Lock()
	h.invites[msg.TargetClientID] = append(h.invites[msg.TargetClientID], invite)
	h.inviteMu.Unlock()

	log.Printf("[Hub] %s invited %s to party %s", client.DisplayName, target.DisplayName, code)

	// Send invite to target
	h.sendMessage(target.Conn, &models.PartyInviteMessage{
		Type:         models.MsgTypePartyInvite,
		FromClientID: client.ID,
		FromName:     client.DisplayName,
		PartyCode:    code,
	})
}

// handleAcceptInvite accepts a party invite
func (h *Hub) handleAcceptInvite(client *models.Client, msg *models.AcceptInviteMessage) {
	h.inviteMu.Lock()
	invites := h.invites[client.ID]
	var foundInvite *models.PartyInvite
	var remaining []*models.PartyInvite

	for _, inv := range invites {
		if inv.PartyCode == msg.PartyCode && inv.ExpiresAt.After(time.Now()) {
			foundInvite = inv
		} else if inv.ExpiresAt.After(time.Now()) {
			remaining = append(remaining, inv)
		}
	}

	if len(remaining) > 0 {
		h.invites[client.ID] = remaining
	} else {
		delete(h.invites, client.ID)
	}
	h.inviteMu.Unlock()

	if foundInvite == nil {
		h.sendError(client.Conn, models.ErrCodeInviteNotFound, "Invite not found or expired")
		return
	}

	// Join the party
	h.handleJoinParty(client, &models.JoinPartyMessage{
		Type:      models.MsgTypeJoinParty,
		PartyCode: msg.PartyCode,
	})

	// Notify inviter
	h.mu.RLock()
	inviter, exists := h.clients[foundInvite.FromClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(inviter.Conn, &models.InviteAcceptedMessage{
			Type:        models.MsgTypeInviteAccepted,
			ClientID:    client.ID,
			DisplayName: client.DisplayName,
		})
	}
}

// handleDeclineInvite declines a party invite
func (h *Hub) handleDeclineInvite(client *models.Client, msg *models.DeclineInviteMessage) {
	h.inviteMu.Lock()
	invites := h.invites[client.ID]
	var foundInvite *models.PartyInvite
	var remaining []*models.PartyInvite

	for _, inv := range invites {
		if inv.PartyCode == msg.PartyCode {
			foundInvite = inv
		} else if inv.ExpiresAt.After(time.Now()) {
			remaining = append(remaining, inv)
		}
	}

	if len(remaining) > 0 {
		h.invites[client.ID] = remaining
	} else {
		delete(h.invites, client.ID)
	}
	h.inviteMu.Unlock()

	if foundInvite == nil {
		return // Silently ignore
	}

	// Notify inviter
	h.mu.RLock()
	inviter, exists := h.clients[foundInvite.FromClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(inviter.Conn, &models.InviteDeclinedMessage{
			Type:        models.MsgTypeInviteDeclined,
			ClientID:    client.ID,
			DisplayName: client.DisplayName,
		})
	}
}

// handleCancelInvite cancels a sent party invite
func (h *Hub) handleCancelInvite(client *models.Client, msg *models.CancelInviteMessage) {
	code := client.GetParty()
	if code == "" {
		h.sendError(client.Conn, models.ErrCodeNotInParty, "Must be in a party to cancel invite")
		return
	}

	h.inviteMu.Lock()
	invites := h.invites[msg.TargetClientID]
	var foundInvite *models.PartyInvite
	var remaining []*models.PartyInvite

	for _, inv := range invites {
		// Find invite from this client for this party
		if inv.FromClientID == client.ID && inv.PartyCode == code {
			foundInvite = inv
		} else if inv.ExpiresAt.After(time.Now()) {
			remaining = append(remaining, inv)
		}
	}

	if len(remaining) > 0 {
		h.invites[msg.TargetClientID] = remaining
	} else {
		delete(h.invites, msg.TargetClientID)
	}
	h.inviteMu.Unlock()

	if foundInvite == nil {
		h.sendError(client.Conn, models.ErrCodeInviteNotFound, "Invite not found")
		return
	}

	log.Printf("[Hub] %s cancelled invite to %s", client.DisplayName, msg.TargetClientID[:8])

	// Notify the target user that the invite was cancelled
	h.mu.RLock()
	target, exists := h.clients[msg.TargetClientID]
	h.mu.RUnlock()

	if exists {
		h.sendMessage(target.Conn, &models.InviteCancelledMessage{
			Type:         models.MsgTypeInviteCancelled,
			FromClientID: client.ID,
			PartyCode:    code,
		})
	}
}

// handleDisconnect cleans up when a client disconnects
func (h *Hub) handleDisconnect(client *models.Client) {
	h.mu.Lock()
	delete(h.clients, client.ID)
	h.mu.Unlock()

	// Leave party if in one
	if code := client.GetParty(); code != "" {
		h.handleLeaveParty(client)
	}

	log.Printf("[Hub] Client disconnected: %s (%s)", client.DisplayName, client.ID[:8])

	// Notify friends that this client is offline
	go h.notifyFriendsOffline(client)
}

// notifyFriendsOnline notifies friends that a client came online
func (h *Hub) notifyFriendsOnline(client *models.Client) {
	friends, err := h.storage.GetFriends(client.ID)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, f := range friends {
		if friend, exists := h.clients[f.FriendID]; exists {
			h.sendMessage(friend.Conn, &models.FriendOnlineMessage{
				Type:        models.MsgTypeFriendOnline,
				ClientID:    client.ID,
				DisplayName: client.DisplayName,
			})
		}
	}
}

// notifyFriendsOffline notifies friends that a client went offline
func (h *Hub) notifyFriendsOffline(client *models.Client) {
	friends, err := h.storage.GetFriends(client.ID)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, f := range friends {
		if friend, exists := h.clients[f.FriendID]; exists {
			h.sendMessage(friend.Conn, &models.FriendOfflineMessage{
				Type:     models.MsgTypeFriendOffline,
				ClientID: client.ID,
			})
		}
	}
}

// generatePartyCode generates a unique party code
func (h *Hub) generatePartyCode() string {
	word := natoAlphabet[rand.Intn(len(natoAlphabet))]
	num := rand.Intn(9000) + 1000 // 1000-9999
	return word + "-" + string(rune('0'+num/1000)) + string(rune('0'+(num%1000)/100)) + string(rune('0'+(num%100)/10)) + string(rune('0'+num%10))
}

// sendMessage sends a message to a connection (safely handles nil connections)
func (h *Hub) sendMessage(conn *websocket.Conn, msg interface{}) {
	if conn == nil {
		return // Test clients have nil connections, silently skip
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[Hub] Failed to marshal message: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[Hub] Failed to send message: %v", err)
	}
}

// sendError sends an error message to a connection
func (h *Hub) sendError(conn *websocket.Conn, code, message string) {
	if conn == nil {
		return // Test clients have nil connections, silently skip
	}
	h.sendMessage(conn, &models.ErrorMessage{
		Type:    models.MsgTypeError,
		Code:    code,
		Message: message,
	})
}

// GetStats returns server statistics
func (h *Hub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	testCount := 0
	for _, c := range h.clients {
		if c.IsTest {
			testCount++
		}
	}

	return map[string]interface{}{
		"clients":     len(h.clients),
		"parties":     len(h.parties),
		"testClients": testCount,
	}
}

// ========== Admin/Dev Methods ==========

// ClientInfo represents client info for admin API
type ClientInfo struct {
	ID          string `json:"clientId"`
	DisplayName string `json:"displayName"`
	RemoteID    string `json:"remoteId,omitempty"`
	PartyCode   string `json:"partyCode,omitempty"`
	CurrentMap  string `json:"currentMap,omitempty"`
	IsTest      bool   `json:"isTest"`
	ConnectedAt string `json:"connectedAt"`
}

// PartyInfo represents party info for admin API
type PartyInfo struct {
	Code        string       `json:"code"`
	HostID      string       `json:"hostId"`
	MemberCount int          `json:"memberCount"`
	Members     []ClientInfo `json:"members"`
	CreatedAt   string       `json:"createdAt"`
}

// GetAllClients returns info about all connected clients (sorted by display name for stability)
func (h *Hub) GetAllClients() []ClientInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, ClientInfo{
			ID:          c.ID,
			DisplayName: c.DisplayName,
			RemoteID:    c.RemoteID,
			PartyCode:   c.GetParty(),
			CurrentMap:  c.CurrentMap,
			IsTest:      c.IsTest,
			ConnectedAt: c.ConnectedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	// Sort by display name for stable ordering (prevents UI jumping)
	sort.Slice(clients, func(i, j int) bool {
		return strings.ToLower(clients[i].DisplayName) < strings.ToLower(clients[j].DisplayName)
	})

	return clients
}

// GetAllParties returns info about all active parties (sorted by code for stability)
func (h *Hub) GetAllParties() []PartyInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	parties := make([]PartyInfo, 0, len(h.parties))
	for _, p := range h.parties {
		members := make([]ClientInfo, 0)
		for _, m := range p.GetMembers() {
			members = append(members, ClientInfo{
				ID:          m.ID,
				DisplayName: m.DisplayName,
				RemoteID:    m.RemoteID,
				IsTest:      m.IsTest,
			})
		}
		// Sort members by display name
		sort.Slice(members, func(i, j int) bool {
			return strings.ToLower(members[i].DisplayName) < strings.ToLower(members[j].DisplayName)
		})

		parties = append(parties, PartyInfo{
			Code:        p.Code,
			HostID:      p.GetHost(),
			MemberCount: p.MemberCount(),
			Members:     members,
			CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	// Sort parties by code for stable ordering
	sort.Slice(parties, func(i, j int) bool {
		return parties[i].Code < parties[j].Code
	})

	return parties
}

// CreateTestClient creates a virtual test client
func (h *Hub) CreateTestClient(displayName, clientID, remoteID string) (*models.Client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clientID == "" {
		clientID = generateTestClientID()
	}
	if displayName == "" {
		displayName = "TestUser_" + clientID[:8]
	}
	if remoteID == "" {
		remoteID = "test-remote-" + clientID[:8]
	}

	// Check if already exists
	if _, exists := h.clients[clientID]; exists {
		return nil, fmt.Errorf("client already exists: %s", clientID[:8])
	}

	// Store in database
	if err := h.storage.RegisterClient(clientID, displayName); err != nil {
		log.Printf("[Hub] Failed to store test client: %v", err)
	}

	client := &models.Client{
		ID:          clientID,
		RemoteID:    remoteID,
		DisplayName: displayName,
		Conn:        nil, // No real connection for test clients
		ConnectedAt: time.Now(),
		IsTest:      true,
	}

	h.clients[clientID] = client
	log.Printf("[Hub] Test client created: %s (%s)", displayName, clientID[:8])

	return client, nil
}

// RemoveTestClient removes a test client
func (h *Hub) RemoveTestClient(clientID string) error {
	h.mu.Lock()
	client, exists := h.clients[clientID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("client not found")
	}
	if !client.IsTest {
		h.mu.Unlock()
		return fmt.Errorf("cannot remove non-test client")
	}
	delete(h.clients, clientID)
	h.mu.Unlock()

	// Leave party if in one
	if code := client.GetParty(); code != "" {
		h.mu.Lock()
		if party, exists := h.parties[code]; exists {
			party.RemoveMember(clientID)
			if party.IsEmpty() {
				delete(h.parties, code)
			}
		}
		h.mu.Unlock()
	}

	log.Printf("[Hub] Test client removed: %s", clientID[:8])
	return nil
}

// CleanupTestClients removes all test clients
func (h *Hub) CleanupTestClients() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	count := 0
	for id, client := range h.clients {
		if client.IsTest {
			// Leave party if in one
			if code := client.GetParty(); code != "" {
				if party, exists := h.parties[code]; exists {
					party.RemoveMember(id)
					if party.IsEmpty() {
						delete(h.parties, code)
					}
				}
			}
			delete(h.clients, id)
			count++
		}
	}

	log.Printf("[Hub] Cleaned up %d test clients", count)
	return count
}

// CreateTestParty creates a party for a test client
func (h *Hub) CreateTestParty(hostClientID string, memberIDs []string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	host, exists := h.clients[hostClientID]
	if !exists {
		return "", fmt.Errorf("host client not found")
	}

	if host.GetParty() != "" {
		return "", fmt.Errorf("host already in a party")
	}

	// Generate unique party code
	var code string
	for {
		code = h.generatePartyCode()
		if _, exists := h.parties[code]; !exists {
			break
		}
	}

	// Create party
	party := models.NewParty(code, hostClientID)
	party.AddMember(host)
	h.parties[code] = party

	log.Printf("[Hub] Test party created: %s by %s", code, host.DisplayName)

	// Add additional members
	for _, memberID := range memberIDs {
		if member, exists := h.clients[memberID]; exists {
			if member.GetParty() == "" {
				party.AddMember(member)
				log.Printf("[Hub] Test member joined: %s -> %s", member.DisplayName, code)
			}
		}
	}

	return code, nil
}

// JoinTestParty adds a client to a party and notifies other members
func (h *Hub) JoinTestParty(clientID, partyCode string) error {
	h.mu.Lock()

	client, exists := h.clients[clientID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("client not found")
	}

	if client.GetParty() != "" {
		h.mu.Unlock()
		return fmt.Errorf("client already in a party")
	}

	party, exists := h.parties[partyCode]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("party not found")
	}

	if !party.AddMember(client) {
		h.mu.Unlock()
		return fmt.Errorf("party is full")
	}

	// Get members list before unlocking
	members := party.GetMembers()
	h.mu.Unlock()

	log.Printf("[Hub] %s joined party %s", client.DisplayName, partyCode)

	// Notify other members that this client joined (including real clients!)
	for _, m := range members {
		if m.ID != client.ID && m.Conn != nil {
			h.sendMessage(m.Conn, &models.MemberJoinedMessage{
				Type:        models.MsgTypeMemberJoined,
				ClientID:    client.ID,
				RemoteID:    client.RemoteID,
				DisplayName: client.DisplayName,
			})
		}
	}

	return nil
}

// LeaveTestParty removes a test client from their party and notifies other members
func (h *Hub) LeaveTestParty(clientID string) error {
	h.mu.Lock()

	client, exists := h.clients[clientID]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("client not found")
	}

	partyCode := client.GetParty()
	if partyCode == "" {
		h.mu.Unlock()
		return fmt.Errorf("client not in a party")
	}

	party, exists := h.parties[partyCode]
	if !exists {
		client.SetParty("")
		h.mu.Unlock()
		return nil
	}

	// Get members before removing
	members := party.GetMembers()

	// Remove member
	party.RemoveMember(clientID)
	log.Printf("[Hub] %s left party %s", client.DisplayName, partyCode)

	// Clean up empty party
	isEmpty := party.IsEmpty()
	if isEmpty {
		delete(h.parties, partyCode)
		log.Printf("[Hub] Party %s deleted (empty)", partyCode)
	}

	h.mu.Unlock()

	// Notify other members that this client left (including real clients!)
	for _, m := range members {
		if m.ID != clientID && m.Conn != nil {
			h.sendMessage(m.Conn, &models.MemberLeftMessage{
				Type:     models.MsgTypeMemberLeft,
				ClientID: clientID,
			})
		}
	}

	return nil
}

// DeleteParty removes a party and notifies all members
func (h *Hub) DeleteParty(partyCode string) error {
	h.mu.Lock()

	party, exists := h.parties[partyCode]
	if !exists {
		h.mu.Unlock()
		return fmt.Errorf("party not found")
	}

	// Get members before deleting
	members := party.GetMembers()

	// Remove all members from party
	for _, member := range members {
		member.SetParty("")
	}

	delete(h.parties, partyCode)
	log.Printf("[Hub] Party deleted: %s", partyCode)

	h.mu.Unlock()

	// Notify all members that the party was deleted
	for _, m := range members {
		if m.Conn != nil {
			h.sendMessage(m.Conn, &models.PartyLeftMessage{
				Type: models.MsgTypePartyLeft,
			})
		}
	}

	return nil
}

// CreateTestFriendship creates a bidirectional friend relationship
func (h *Hub) CreateTestFriendship(clientID1, clientID2 string) error {
	h.mu.RLock()
	client1, exists1 := h.clients[clientID1]
	client2, exists2 := h.clients[clientID2]
	h.mu.RUnlock()

	if !exists1 || !exists2 {
		return fmt.Errorf("one or both clients not found")
	}

	if err := h.storage.AddFriend(clientID1, clientID2, client2.DisplayName); err != nil {
		return fmt.Errorf("failed to add friend relationship: %w", err)
	}

	log.Printf("[Hub] Test friendship created: %s <-> %s", client1.DisplayName, client2.DisplayName)

	// Notify both clients if they have connections
	if client1.Conn != nil {
		h.sendMessage(client1.Conn, &models.FriendAddedMessage{
			Type:        models.MsgTypeFriendAdded,
			ClientID:    clientID2,
			DisplayName: client2.DisplayName,
		})
	}
	if client2.Conn != nil {
		h.sendMessage(client2.Conn, &models.FriendAddedMessage{
			Type:        models.MsgTypeFriendAdded,
			ClientID:    clientID1,
			DisplayName: client1.DisplayName,
		})
	}

	return nil
}

// RemoveTestFriendship removes a friend relationship
func (h *Hub) RemoveTestFriendship(clientID1, clientID2 string) error {
	if err := h.storage.RemoveFriend(clientID1, clientID2); err != nil {
		return fmt.Errorf("failed to remove friend relationship: %w", err)
	}

	log.Printf("[Hub] Friendship removed: %s <-> %s", clientID1[:8], clientID2[:8])

	h.mu.RLock()
	client1, exists1 := h.clients[clientID1]
	client2, exists2 := h.clients[clientID2]
	h.mu.RUnlock()

	// Notify clients if they're online
	if exists1 && client1.Conn != nil {
		h.sendMessage(client1.Conn, &models.FriendRemovedMessage{
			Type:     models.MsgTypeFriendRemoved,
			ClientID: clientID2,
		})
	}
	if exists2 && client2.Conn != nil {
		h.sendMessage(client2.Conn, &models.FriendRemovedMessage{
			Type:     models.MsgTypeFriendRemoved,
			ClientID: clientID1,
		})
	}

	return nil
}

// GetFriendsForClient returns friends list for a client
func (h *Hub) GetFriendsForClient(clientID string) ([]models.FriendStatus, error) {
	friends, err := h.storage.GetFriends(clientID)
	if err != nil {
		return nil, err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]models.FriendStatus, len(friends))
	for i, f := range friends {
		status := models.FriendStatus{
			ClientID:    f.FriendID,
			DisplayName: f.FriendName,
			Online:      false,
			InParty:     false,
		}

		if onlineClient, exists := h.clients[f.FriendID]; exists {
			status.Online = true
			status.DisplayName = onlineClient.DisplayName
			if partyCode := onlineClient.GetParty(); partyCode != "" {
				status.InParty = true
				status.PartyCode = partyCode
			}
		}

		result[i] = status
	}

	return result, nil
}

// SendTestPosition sends a test position update for a client
func (h *Hub) SendTestPosition(clientID, mapName string, x, y, z, rotation float64) error {
	h.mu.RLock()
	client, exists := h.clients[clientID]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found")
	}

	code := client.GetParty()
	if code == "" {
		return fmt.Errorf("client not in a party")
	}

	// Update client position
	client.UpdatePosition(&models.Position{
		X:        x,
		Y:        y,
		Z:        z,
		Rotation: rotation,
	}, mapName)

	h.mu.RLock()
	party, exists := h.parties[code]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("party not found")
	}

	// Broadcast to other members
	update := &models.PositionUpdateMessage{
		Type:        models.MsgTypePositionUpdate,
		ClientID:    client.ID,
		RemoteID:    client.RemoteID,
		DisplayName: client.DisplayName,
		Map:         mapName,
		Position: models.Position{
			X:        x,
			Y:        y,
			Z:        z,
			Rotation: rotation,
		},
	}

	for _, m := range party.GetMembers() {
		if m.ID != client.ID && m.Conn != nil {
			h.sendMessage(m.Conn, update)
		}
	}

	log.Printf("[Hub] Test position sent: %s on %s (%.1f, %.1f, %.1f)", client.DisplayName, mapName, x, y, z)
	return nil
}

// FindClientByPrefix finds a client by ID prefix
func (h *Hub) FindClientByPrefix(prefix string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	prefix = strings.ToLower(prefix)
	for id := range h.clients {
		if strings.HasPrefix(strings.ToLower(id), prefix) {
			return id
		}
	}
	return ""
}

// generateTestClientID generates a unique test client ID
func generateTestClientID() string {
	// Generate a UUID-like string
	return fmt.Sprintf("test-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
}

// FriendRequestInfo represents a friend request for the admin API
type FriendRequestInfo struct {
	FromClientID   string `json:"fromClientId"`
	FromName       string `json:"fromName"`
	ToClientID     string `json:"toClientId"`
	ToName         string `json:"toName"`
	SentAt         string `json:"sentAt"`
	FromIsTest     bool   `json:"fromIsTest"`
	ToIsTest       bool   `json:"toIsTest"`
}

// GetAllPendingFriendRequests returns all pending friend requests for connected clients
func (h *Hub) GetAllPendingFriendRequests() []FriendRequestInfo {
	h.mu.RLock()
	clientIDs := make([]string, 0, len(h.clients))
	clientTestMap := make(map[string]bool)
	clientNameMap := make(map[string]string)
	for id, c := range h.clients {
		clientIDs = append(clientIDs, id)
		clientTestMap[id] = c.IsTest
		clientNameMap[id] = c.DisplayName
	}
	h.mu.RUnlock()

	// Collect all incoming requests for connected clients
	seen := make(map[string]bool)
	var requests []FriendRequestInfo

	for _, clientID := range clientIDs {
		incoming, err := h.storage.GetIncomingFriendRequests(clientID)
		if err != nil {
			continue
		}
		for _, r := range incoming {
			key := r.ClientID + "->" + clientID
			if seen[key] {
				continue
			}
			seen[key] = true

			requests = append(requests, FriendRequestInfo{
				FromClientID: r.ClientID,
				FromName:     r.DisplayName,
				ToClientID:   clientID,
				ToName:       clientNameMap[clientID],
				SentAt:       r.SentAt.Format("15:04:05"),
				FromIsTest:   clientTestMap[r.ClientID],
				ToIsTest:     clientTestMap[clientID],
			})
		}
	}

	return requests
}

// AcceptFriendRequestForClient accepts a friend request on behalf of a test client
func (h *Hub) AcceptFriendRequestForClient(clientID, fromClientID string) error {
	h.mu.RLock()
	client, exists := h.clients[clientID]
	fromClient, fromExists := h.clients[fromClientID]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found")
	}
	if !client.IsTest {
		return fmt.Errorf("can only accept requests for test clients")
	}

	// Check if request exists
	requestExists, err := h.storage.GetFriendRequest(fromClientID, clientID)
	if err != nil {
		return fmt.Errorf("failed to check friend request: %w", err)
	}
	if !requestExists {
		return fmt.Errorf("friend request not found")
	}

	// Use the existing accept logic
	var fromClientPtr *models.Client
	if fromExists {
		fromClientPtr = fromClient
	}
	h.acceptFriendRequest(client, fromClientID, fromClientPtr)

	return nil
}

// DeclineFriendRequestForClient declines a friend request on behalf of a test client
func (h *Hub) DeclineFriendRequestForClient(clientID, fromClientID string) error {
	h.mu.RLock()
	client, exists := h.clients[clientID]
	fromClient, fromExists := h.clients[fromClientID]
	h.mu.RUnlock()

	if !exists {
		return fmt.Errorf("client not found")
	}
	if !client.IsTest {
		return fmt.Errorf("can only decline requests for test clients")
	}

	// Check if request exists
	requestExists, err := h.storage.GetFriendRequest(fromClientID, clientID)
	if err != nil {
		return fmt.Errorf("failed to check friend request: %w", err)
	}
	if !requestExists {
		return fmt.Errorf("friend request not found")
	}

	// Delete the request
	if err := h.storage.DeleteFriendRequest(fromClientID, clientID); err != nil {
		return fmt.Errorf("failed to decline friend request: %w", err)
	}

	log.Printf("[Hub] %s declined friend request from %s", client.DisplayName, fromClientID[:8])

	// Notify the requester (if online)
	if fromExists && fromClient.Conn != nil {
		h.sendMessage(fromClient.Conn, &models.FriendRequestDeclinedMessage{
			Type:        models.MsgTypeFriendRequestDeclined,
			ClientID:    clientID,
			DisplayName: client.DisplayName,
		})
	}

	return nil
}

// SendFriendRequestForClient sends a friend request from a test client to another client
func (h *Hub) SendFriendRequestForClient(fromClientID, toClientID string) error {
	h.mu.RLock()
	fromClient, fromExists := h.clients[fromClientID]
	toClient, toExists := h.clients[toClientID]
	h.mu.RUnlock()

	if !fromExists {
		return fmt.Errorf("from client not found")
	}
	if !fromClient.IsTest {
		return fmt.Errorf("can only send requests from test clients")
	}
	if !toExists {
		return fmt.Errorf("to client not found")
	}

	// Check if already friends
	areFriends, err := h.storage.AreFriends(fromClientID, toClientID)
	if err != nil {
		return fmt.Errorf("failed to check friendship: %w", err)
	}
	if areFriends {
		return fmt.Errorf("already friends")
	}

	// Check if request already exists
	requestExists, err := h.storage.GetFriendRequest(fromClientID, toClientID)
	if err != nil {
		return fmt.Errorf("failed to check existing request: %w", err)
	}
	if requestExists {
		return fmt.Errorf("friend request already sent")
	}

	// Check if reverse request exists (they sent us one)
	reverseExists, err := h.storage.GetFriendRequest(toClientID, fromClientID)
	if err != nil {
		return fmt.Errorf("failed to check reverse request: %w", err)
	}
	if reverseExists {
		// Auto-accept if they already sent us a request
		h.acceptFriendRequest(fromClient, toClientID, toClient)
		return nil
	}

	// Create the friend request
	if err := h.storage.CreateFriendRequest(fromClientID, toClientID, fromClient.DisplayName, toClient.DisplayName); err != nil {
		return fmt.Errorf("failed to create friend request: %w", err)
	}

	log.Printf("[Hub] %s sent friend request to %s", fromClient.DisplayName, toClient.DisplayName)

	// Notify the target (if they have a connection - real clients)
	if toClient.Conn != nil {
		h.sendMessage(toClient.Conn, &models.FriendRequestMessage{
			Type:         models.MsgTypeFriendRequest,
			FromClientID: fromClientID,
			DisplayName:  fromClient.DisplayName,
		})
	}

	return nil
}
