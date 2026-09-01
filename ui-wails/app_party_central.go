package main

import (
	"crypto/rand"
	"fmt"
	"time"

	"tarkov-screenshot-analyzer/internal/party"
)

// generateUUID creates a random UUID v4 string.
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CentralPartyState holds state for the centralized party system
type CentralPartyState struct {
	client                  *party.CentralClient
	connected               bool
	registered              bool
	inParty                 bool
	partyCode               string
	isHost                  bool
	members                 []map[string]interface{}
	friends                 []map[string]interface{}
	pendingInvites          []map[string]interface{}
	incomingFriendRequests  []map[string]interface{}
	outgoingFriendRequests  []map[string]interface{}
}

// centralPartyState is the global state for centralized party
var centralPartyState = &CentralPartyState{
	members:                 make([]map[string]interface{}, 0),
	friends:                 make([]map[string]interface{}, 0),
	pendingInvites:          make([]map[string]interface{}, 0),
	incomingFriendRequests:  make([]map[string]interface{}, 0),
	outgoingFriendRequests:  make([]map[string]interface{}, 0),
}

// ===== CENTRALIZED PARTY METHODS =====

// ConnectToPartyServer connects to the centralized party server
func (a *App) ConnectToPartyServer(displayNameOverride string) error {
	// Read and update config under lock, then release before network calls
	a.stateMutex.Lock()

	if !a.config.PartySettings.Enabled {
		a.config.PartySettings.Enabled = true
	}
	if a.config.PartySettings.ServerURL == "" {
		a.config.PartySettings.ServerURL = "wss://party.tarkov.nexus/ws"
	}
	if centralPartyState.connected {
		a.stateMutex.Unlock()
		return fmt.Errorf("already connected to party server")
	}

	clientID := a.config.PartySettings.ClientID
	if clientID == "" {
		clientID = generateUUID()
		a.config.PartySettings.ClientID = clientID
		a.logInfo(fmt.Sprintf("Generated party client ID: %s", clientID))
	}

	// Use the display name passed directly from the UI (avoids config timing issues)
	displayName := displayNameOverride
	if displayName == "" {
		displayName = a.config.PartySettings.DisplayName
	}
	if displayName == "" {
		displayName = "Player"
	}
	// Save display name + client ID back to config
	a.config.PartySettings.DisplayName = displayName

	serverURL := a.config.PartySettings.ServerURL
	remoteID := a.config.RemoteID
	a.stateMutex.Unlock()

	// Persist to disk outside the lock
	if err := a.SaveConfig(a.config); err != nil {
		a.logError(fmt.Sprintf("Failed to save party config: %v", err))
	}

	// Create client and connect (no lock held — these do network I/O)
	client := party.NewCentralClient(serverURL, a)
	client.SetEventHandler(&centralPartyEventHandler{app: a})

	a.logInfo(fmt.Sprintf("Connecting to party server: %s", a.config.PartySettings.ServerURL))

	if err := client.Connect(clientID, remoteID, displayName); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	centralPartyState.client = client
	centralPartyState.connected = true

	a.logInfo("Connected to centralized party server")
	a.emitEvent("centralParty:connected", nil)

	return nil
}

// DisconnectFromPartyServer disconnects from the centralized party server
func (a *App) DisconnectFromPartyServer() error {
	a.stateMutex.Lock()
	defer a.stateMutex.Unlock()

	if centralPartyState.client == nil || !centralPartyState.connected {
		return fmt.Errorf("not connected to party server")
	}

	if err := centralPartyState.client.Disconnect(); err != nil {
		a.logError(fmt.Sprintf("Error disconnecting: %v", err))
	}

	centralPartyState.client = nil
	centralPartyState.connected = false
	centralPartyState.registered = false
	centralPartyState.inParty = false
	centralPartyState.partyCode = ""
	centralPartyState.isHost = false
	centralPartyState.members = make([]map[string]interface{}, 0)

	a.logInfo("Disconnected from centralized party server")
	a.emitEvent("centralParty:disconnected", nil)

	return nil
}

// CreateCentralParty creates a new party on the centralized server
func (a *App) CreateCentralParty() (string, error) {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return "", fmt.Errorf("not connected to party server")
	}

	if centralPartyState.inParty {
		return "", fmt.Errorf("already in a party")
	}

	a.logInfo("Creating party...")

	if err := centralPartyState.client.CreateParty(); err != nil {
		return "", err
	}

	// Wait briefly for server response
	time.Sleep(500 * time.Millisecond)

	return centralPartyState.partyCode, nil
}

// JoinCentralParty joins a party on the centralized server
func (a *App) JoinCentralParty(partyCode string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	if centralPartyState.inParty {
		return fmt.Errorf("already in a party - leave first")
	}

	a.logInfo(fmt.Sprintf("Joining party: %s", partyCode))

	return centralPartyState.client.JoinParty(partyCode)
}

// LeaveCentralParty leaves the current party
func (a *App) LeaveCentralParty() error {
	if centralPartyState.client == nil || !centralPartyState.inParty {
		return fmt.Errorf("not in a party")
	}

	a.logInfo("Leaving party...")

	return centralPartyState.client.LeaveParty()
}

// GetCentralPartyStatus returns the current centralized party status
func (a *App) GetCentralPartyStatus() map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	return map[string]interface{}{
		"connected":   centralPartyState.connected,
		"registered":  centralPartyState.registered,
		"inParty":     centralPartyState.inParty,
		"partyCode":   centralPartyState.partyCode,
		"isHost":      centralPartyState.isHost,
		"memberCount": len(centralPartyState.members),
		"serverUrl":   a.config.PartySettings.ServerURL,
		"clientId":    a.config.PartySettings.ClientID,
	}
}

// GetCentralPartyMembers returns the list of party members
func (a *App) GetCentralPartyMembers() []map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	return centralPartyState.members
}

// GetFriends requests and returns the friends list
func (a *App) GetFriends() ([]map[string]interface{}, error) {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return nil, fmt.Errorf("not connected to party server")
	}

	if err := centralPartyState.client.GetFriends(); err != nil {
		return nil, err
	}

	// Return cached friends (will be updated by event)
	return centralPartyState.friends, nil
}

// AddFriend adds a friend by client ID
func (a *App) AddFriend(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	return centralPartyState.client.AddFriend(targetClientID)
}

// RemoveFriend removes a friend
func (a *App) RemoveFriend(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	return centralPartyState.client.RemoveFriend(targetClientID)
}

// AcceptFriendRequest accepts a friend request
func (a *App) AcceptFriendRequest(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	a.logInfo(fmt.Sprintf("Accepting friend request from: %s", shortID(targetClientID)))

	return centralPartyState.client.AcceptFriendRequest(targetClientID)
}

// DeclineFriendRequest declines a friend request
func (a *App) DeclineFriendRequest(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	a.logInfo(fmt.Sprintf("Declining friend request from: %s", shortID(targetClientID)))

	// Remove from local pending list immediately
	removeIncomingFriendRequest(targetClientID)

	return centralPartyState.client.DeclineFriendRequest(targetClientID)
}

// CancelFriendRequest cancels an outgoing friend request
func (a *App) CancelFriendRequest(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	a.logInfo(fmt.Sprintf("Cancelling friend request to: %s", shortID(targetClientID)))

	// Remove from local pending list immediately
	removeOutgoingFriendRequest(targetClientID)

	return centralPartyState.client.CancelFriendRequest(targetClientID)
}

// GetPendingFriendRequests requests the list of pending friend requests
func (a *App) GetPendingFriendRequests() error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	return centralPartyState.client.GetPendingFriendRequests()
}

// GetIncomingFriendRequests returns cached incoming friend requests
func (a *App) GetIncomingFriendRequests() []map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	return centralPartyState.incomingFriendRequests
}

// GetOutgoingFriendRequests returns cached outgoing friend requests
func (a *App) GetOutgoingFriendRequests() []map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	return centralPartyState.outgoingFriendRequests
}

// removeIncomingFriendRequest removes an incoming friend request from the local list
func removeIncomingFriendRequest(clientID string) {
	newRequests := make([]map[string]interface{}, 0)
	for _, req := range centralPartyState.incomingFriendRequests {
		if req["clientId"] != clientID {
			newRequests = append(newRequests, req)
		}
	}
	centralPartyState.incomingFriendRequests = newRequests
}

// removeOutgoingFriendRequest removes an outgoing friend request from the local list
func removeOutgoingFriendRequest(clientID string) {
	newRequests := make([]map[string]interface{}, 0)
	for _, req := range centralPartyState.outgoingFriendRequests {
		if req["clientId"] != clientID {
			newRequests = append(newRequests, req)
		}
	}
	centralPartyState.outgoingFriendRequests = newRequests
}

// InviteFriendToParty invites a friend to the current party
func (a *App) InviteFriendToParty(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.inParty {
		return fmt.Errorf("not in a party")
	}

	a.logInfo(fmt.Sprintf("Inviting friend to party: %s", targetClientID))

	return centralPartyState.client.InviteFriend(targetClientID)
}

// AcceptPartyInvite accepts a party invitation
func (a *App) AcceptPartyInvite(partyCode string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	a.logInfo(fmt.Sprintf("Accepting invite to party: %s", partyCode))

	// Remove the invite from pending list
	removeInviteFromPending(partyCode)

	return centralPartyState.client.AcceptInvite(partyCode)
}

// DeclinePartyInvite declines a party invitation
func (a *App) DeclinePartyInvite(partyCode string) error {
	if centralPartyState.client == nil || !centralPartyState.registered {
		return fmt.Errorf("not connected to party server")
	}

	// Remove the invite from pending list
	removeInviteFromPending(partyCode)

	return centralPartyState.client.DeclineInvite(partyCode)
}

// CancelPartyInvite cancels an outgoing party invitation
func (a *App) CancelPartyInvite(targetClientID string) error {
	if centralPartyState.client == nil || !centralPartyState.inParty {
		return fmt.Errorf("not in a party")
	}

	a.logInfo(fmt.Sprintf("Cancelling invite to: %s", shortID(targetClientID)))

	return centralPartyState.client.CancelInvite(targetClientID)
}

// removeInviteFromPending removes an invite from the pending list by party code
func removeInviteFromPending(partyCode string) {
	newInvites := make([]map[string]interface{}, 0)
	for _, invite := range centralPartyState.pendingInvites {
		if invite["partyCode"] != partyCode {
			newInvites = append(newInvites, invite)
		}
	}
	centralPartyState.pendingInvites = newInvites
}

// GetPendingInvites returns pending party invitations
func (a *App) GetPendingInvites() []map[string]interface{} {
	a.stateMutex.RLock()
	defer a.stateMutex.RUnlock()

	return centralPartyState.pendingInvites
}

// SendPositionToParty sends position update to party members
func (a *App) SendPositionToParty(mapName string, x, y, z, rotation float64) error {
	if centralPartyState.client == nil || !centralPartyState.inParty {
		return nil // Silently ignore if not in party
	}

	pos := &party.Position{
		X:        x,
		Y:        y,
		Z:        z,
		Rotation: rotation,
	}

	return centralPartyState.client.SendPosition(mapName, pos)
}

// ===== EVENT HANDLER =====

type centralPartyEventHandler struct {
	app *App
}

func (h *centralPartyEventHandler) OnRegistered(clientID string) {
	centralPartyState.registered = true
	h.app.logInfo(fmt.Sprintf("Registered with party server (ID: %s)", shortID(clientID)))
	h.app.emitEvent("centralParty:registered", map[string]string{"clientId": clientID})
}

func (h *centralPartyEventHandler) OnPartyCreated(partyCode string) {
	centralPartyState.inParty = true
	centralPartyState.partyCode = partyCode
	centralPartyState.isHost = true
	centralPartyState.members = []map[string]interface{}{
		{
			"clientId":    h.app.config.PartySettings.ClientID,
			"displayName": h.app.config.PartySettings.DisplayName,
			"isHost":      true,
		},
	}
	h.app.logInfo(fmt.Sprintf("Party created: %s", partyCode))
	h.app.emitEvent("centralParty:created", map[string]string{"partyCode": partyCode})
}

func (h *centralPartyEventHandler) OnPartyJoined(partyCode string, members []party.CentralMemberInfo) {
	centralPartyState.inParty = true
	centralPartyState.partyCode = partyCode

	// Clear any pending invites for this party (we just joined it)
	removeInviteFromPending(partyCode)

	// Convert members
	centralPartyState.members = make([]map[string]interface{}, len(members))
	for i, m := range members {
		member := map[string]interface{}{
			"clientId":    m.ClientID,
			"remoteId":    m.RemoteID,
			"displayName": m.DisplayName,
			"currentMap":  m.CurrentMap,
			"isHost":      m.IsHost,
		}
		if m.Position != nil {
			member["position"] = map[string]float64{
				"x":        m.Position.X,
				"y":        m.Position.Y,
				"z":        m.Position.Z,
				"rotation": m.Position.Rotation,
			}
		}
		centralPartyState.members[i] = member

		if m.IsHost && m.ClientID == h.app.config.PartySettings.ClientID {
			centralPartyState.isHost = true
		}
	}

	h.app.logInfo(fmt.Sprintf("Joined party: %s (%d members)", partyCode, len(members)))
	h.app.emitEvent("centralParty:joined", map[string]interface{}{
		"partyCode": partyCode,
		"members":   centralPartyState.members,
	})
}

func (h *centralPartyEventHandler) OnPartyLeft() {
	centralPartyState.inParty = false
	centralPartyState.partyCode = ""
	centralPartyState.isHost = false
	centralPartyState.members = make([]map[string]interface{}, 0)
	h.app.logInfo("Left party")
	h.app.emitEvent("centralParty:left", nil)
}

func (h *centralPartyEventHandler) OnMemberJoined(clientID, remoteID, displayName string) {
	member := map[string]interface{}{
		"clientId":    clientID,
		"remoteId":    remoteID,
		"displayName": displayName,
		"isHost":      false,
	}
	centralPartyState.members = append(centralPartyState.members, member)
	h.app.logInfo(fmt.Sprintf("Member joined: %s", displayName))
	h.app.emitEvent("centralParty:memberJoined", member)
}

func (h *centralPartyEventHandler) OnMemberLeft(clientID string) {
	// Remove member from list
	newMembers := make([]map[string]interface{}, 0)
	for _, m := range centralPartyState.members {
		if m["clientId"] != clientID {
			newMembers = append(newMembers, m)
		}
	}
	centralPartyState.members = newMembers
	h.app.logInfo(fmt.Sprintf("Member left: %s", shortID(clientID)))
	h.app.emitEvent("centralParty:memberLeft", map[string]string{"clientId": clientID})
}

func (h *centralPartyEventHandler) OnPositionUpdate(clientID, remoteID, displayName, mapName string, pos *party.Position) {
	// Update member in list
	for i, m := range centralPartyState.members {
		if m["clientId"] == clientID {
			centralPartyState.members[i]["currentMap"] = mapName
			centralPartyState.members[i]["displayName"] = displayName
			centralPartyState.members[i]["remoteId"] = remoteID
			centralPartyState.members[i]["position"] = map[string]float64{
				"x":        pos.X,
				"y":        pos.Y,
				"z":        pos.Z,
				"rotation": pos.Rotation,
			}
			centralPartyState.members[i]["lastUpdate"] = time.Now().Format(time.RFC3339)
			break
		}
	}

	// Party positions are rendered by the local overlay server only. They are
	// deliberately not sent to tarkov.dev — see internal/websocket/client.go.
	h.app.stateMutex.RLock()
	overlayServer := h.app.overlayServer
	h.app.stateMutex.RUnlock()

	// Broadcast to local overlay server for userscript markers
	if overlayServer != nil {
		overlayServer.Broadcast(map[string]interface{}{
			"type": "partyPosition",
			"data": map[string]interface{}{
				"remoteId":    remoteID,
				"displayName": displayName,
				"position": map[string]interface{}{
					"x": pos.X,
					"y": pos.Y,
					"z": pos.Z,
				},
				"rotation": pos.Rotation,
				"map":      mapName,
			},
		})
	}

	h.app.emitEvent("centralParty:positionUpdate", map[string]interface{}{
		"clientId":    clientID,
		"remoteId":    remoteID,
		"displayName": displayName,
		"map":         mapName,
		"position": map[string]float64{
			"x":        pos.X,
			"y":        pos.Y,
			"z":        pos.Z,
			"rotation": pos.Rotation,
		},
	})
}

func (h *centralPartyEventHandler) OnFriendsList(friends []party.FriendStatus) {
	centralPartyState.friends = make([]map[string]interface{}, len(friends))
	for i, f := range friends {
		centralPartyState.friends[i] = map[string]interface{}{
			"clientId":    f.ClientID,
			"displayName": f.DisplayName,
			"online":      f.Online,
			"inParty":     f.InParty,
			"partyCode":   f.PartyCode,
		}
	}
	h.app.emitEvent("centralParty:friendsList", centralPartyState.friends)
}

func (h *centralPartyEventHandler) OnFriendOnline(clientID, displayName string) {
	// Update friend in list
	for i, f := range centralPartyState.friends {
		if f["clientId"] == clientID {
			centralPartyState.friends[i]["online"] = true
			centralPartyState.friends[i]["displayName"] = displayName
			break
		}
	}
	h.app.logInfo(fmt.Sprintf("Friend online: %s", displayName))
	h.app.emitEvent("centralParty:friendOnline", map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
	})
}

func (h *centralPartyEventHandler) OnFriendOffline(clientID string) {
	// Update friend in list
	for i, f := range centralPartyState.friends {
		if f["clientId"] == clientID {
			centralPartyState.friends[i]["online"] = false
			centralPartyState.friends[i]["inParty"] = false
			break
		}
	}
	h.app.emitEvent("centralParty:friendOffline", map[string]string{"clientId": clientID})
}

func (h *centralPartyEventHandler) OnFriendAdded(clientID, displayName string) {
	friend := map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
		"online":      true, // They must be online to be added
		"inParty":     false,
	}
	centralPartyState.friends = append(centralPartyState.friends, friend)
	h.app.logInfo(fmt.Sprintf("Friend added: %s", displayName))
	h.app.emitEvent("centralParty:friendAdded", friend)
}

func (h *centralPartyEventHandler) OnFriendRemoved(clientID string) {
	// Remove from list
	newFriends := make([]map[string]interface{}, 0)
	for _, f := range centralPartyState.friends {
		if f["clientId"] != clientID {
			newFriends = append(newFriends, f)
		}
	}
	centralPartyState.friends = newFriends
	h.app.emitEvent("centralParty:friendRemoved", map[string]string{"clientId": clientID})
}

func (h *centralPartyEventHandler) OnFriendRequest(fromClientID, displayName string) {
	// Check if we already have a pending request from this user
	for _, existing := range centralPartyState.incomingFriendRequests {
		if existing["clientId"] == fromClientID {
			h.app.logDebug(fmt.Sprintf("Ignoring duplicate friend request from %s", displayName))
			return
		}
	}

	request := map[string]interface{}{
		"clientId":    fromClientID,
		"displayName": displayName,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	centralPartyState.incomingFriendRequests = append(centralPartyState.incomingFriendRequests, request)
	h.app.logInfo(fmt.Sprintf("Friend request from %s", displayName))
	h.app.emitEvent("centralParty:friendRequest", request)
}

func (h *centralPartyEventHandler) OnFriendRequestSent(toClientID, displayName string) {
	// Add to outgoing requests
	request := map[string]interface{}{
		"clientId":    toClientID,
		"displayName": displayName,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
	centralPartyState.outgoingFriendRequests = append(centralPartyState.outgoingFriendRequests, request)
	h.app.logInfo(fmt.Sprintf("Friend request sent to %s", displayName))
	h.app.emitEvent("centralParty:friendRequestSent", request)
}

func (h *centralPartyEventHandler) OnFriendRequestAccepted(clientID, displayName string) {
	// Remove from outgoing requests (they accepted our request)
	removeOutgoingFriendRequest(clientID)

	h.app.logInfo(fmt.Sprintf("%s accepted your friend request", displayName))
	h.app.emitEvent("centralParty:friendRequestAccepted", map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
	})
}

func (h *centralPartyEventHandler) OnFriendRequestDeclined(clientID, displayName string) {
	// Remove from outgoing requests (they declined our request)
	removeOutgoingFriendRequest(clientID)

	h.app.logInfo(fmt.Sprintf("%s declined your friend request", displayName))
	h.app.emitEvent("centralParty:friendRequestDeclined", map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
	})
}

func (h *centralPartyEventHandler) OnFriendRequestCancelled(fromClientID string) {
	// Remove from incoming requests (they cancelled their request)
	removeIncomingFriendRequest(fromClientID)

	h.app.logInfo("Friend request was cancelled")
	h.app.emitEvent("centralParty:friendRequestCancelled", map[string]string{
		"fromClientId": fromClientID,
	})
}

func (h *centralPartyEventHandler) OnFriendRequestsList(incoming, outgoing []party.FriendRequestInfo) {
	// Update incoming requests
	centralPartyState.incomingFriendRequests = make([]map[string]interface{}, len(incoming))
	for i, req := range incoming {
		centralPartyState.incomingFriendRequests[i] = map[string]interface{}{
			"clientId":    req.ClientID,
			"displayName": req.DisplayName,
			"sentAt":      req.SentAt,
		}
	}

	// Update outgoing requests
	centralPartyState.outgoingFriendRequests = make([]map[string]interface{}, len(outgoing))
	for i, req := range outgoing {
		centralPartyState.outgoingFriendRequests[i] = map[string]interface{}{
			"clientId":    req.ClientID,
			"displayName": req.DisplayName,
			"sentAt":      req.SentAt,
		}
	}

	h.app.emitEvent("centralParty:friendRequestsList", map[string]interface{}{
		"incoming": centralPartyState.incomingFriendRequests,
		"outgoing": centralPartyState.outgoingFriendRequests,
	})
}

func (h *centralPartyEventHandler) OnPartyInvite(fromClientID, fromName, partyCode string) {
	// Check if we already have a pending invite from this user or for this party
	for _, existing := range centralPartyState.pendingInvites {
		// Skip if we already have an invite from this user
		if existing["fromClientId"] == fromClientID {
			h.app.logDebug(fmt.Sprintf("Ignoring duplicate invite from %s", fromName))
			return
		}
		// Skip if we already have an invite for this party
		if existing["partyCode"] == partyCode {
			h.app.logDebug(fmt.Sprintf("Ignoring duplicate invite for party %s", partyCode))
			return
		}
	}

	invite := map[string]interface{}{
		"fromClientId":    fromClientID,
		"fromDisplayName": fromName,
		"partyCode":       partyCode,
		"timestamp":       time.Now().Format(time.RFC3339),
	}
	centralPartyState.pendingInvites = append(centralPartyState.pendingInvites, invite)
	h.app.logInfo(fmt.Sprintf("Party invite from %s", fromName))
	h.app.emitEvent("centralParty:invite", invite)
}

func (h *centralPartyEventHandler) OnInviteAccepted(clientID, displayName string) {
	h.app.logInfo(fmt.Sprintf("%s accepted invite", displayName))
	h.app.emitEvent("centralParty:inviteAccepted", map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
	})
}

func (h *centralPartyEventHandler) OnInviteDeclined(clientID, displayName string) {
	h.app.logInfo(fmt.Sprintf("%s declined invite", displayName))
	h.app.emitEvent("centralParty:inviteDeclined", map[string]interface{}{
		"clientId":    clientID,
		"displayName": displayName,
	})
}

func (h *centralPartyEventHandler) OnInviteCancelled(fromClientID, partyCode string) {
	// Remove the invite from pending list when the sender cancels it
	removeInviteFromPending(partyCode)
	h.app.logInfo("Party invite was cancelled")
	h.app.emitEvent("centralParty:inviteCancelled", map[string]string{
		"fromClientId": fromClientID,
		"partyCode":    partyCode,
	})
}

func (h *centralPartyEventHandler) OnError(code, message string) {
	h.app.logError(fmt.Sprintf("Party error [%s]: %s", code, message))
	h.app.emitEvent("centralParty:error", map[string]string{
		"code":    code,
		"message": message,
	})
}

func (h *centralPartyEventHandler) OnDisconnected() {
	centralPartyState.connected = false
	centralPartyState.registered = false
	centralPartyState.inParty = false
	centralPartyState.partyCode = ""
	centralPartyState.isHost = false
	centralPartyState.members = make([]map[string]interface{}, 0)
	h.app.logWarning("Disconnected from party server")
	h.app.emitEvent("centralParty:disconnected", nil)
}
