package party

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"tarkov-screenshot-analyzer/internal/logger"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for local development
		// TODO: Restrict to localhost in production
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Server manages the party WebSocket server
type Server struct {
	registry      *PartyRegistry
	logger        logger.Logger
	port          int
	httpServer    *http.Server
	clients       map[*websocket.Conn]*ClientInfo
	clientsMutex  sync.RWMutex
	rateLimiters  map[string]*RateLimiter
	rateLimiterMu sync.RWMutex
	cleanupTicker *time.Ticker
	ctx           context.Context
	cancel        context.CancelFunc
}

// ClientInfo stores information about a connected client
type ClientInfo struct {
	RemoteID    string
	PartyCode   string
	DisplayName string
	Conn        *websocket.Conn
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter (1 message per second)
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		tokens:     10,
		maxTokens:  10,
		refillRate: time.Second,
		lastRefill: time.Now(),
	}
}

// Allow checks if an action is allowed under rate limiting
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)

	if tokensToAdd > 0 {
		rl.tokens = min(rl.tokens+tokensToAdd, rl.maxTokens)
		rl.lastRefill = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NewServer creates a new party server
func NewServer(port int, log logger.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		registry:     NewPartyRegistry(),
		logger:       log,
		port:         port,
		clients:      make(map[*websocket.Conn]*ClientInfo),
		rateLimiters: make(map[string]*RateLimiter),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the party server
func (s *Server) Start() error {
	// Check if port is available
	if !IsPortAvailable(s.port) {
		return fmt.Errorf("port %d is already in use - please close the application using it or change the party server port in settings", s.port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/party", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Start cleanup routine for inactive parties
	s.cleanupTicker = time.NewTicker(1 * time.Hour)
	go s.cleanupRoutine()

	// Get preferred IP for LAN access
	preferredIP, _ := GetPreferredIP()
	if preferredIP != "" {
		s.logger.Info(fmt.Sprintf("🎮 Party server starting on port %d", s.port))
		s.logger.Info(fmt.Sprintf("📡 Local network URL: ws://%s:%d/party", preferredIP, s.port))
	} else {
		s.logger.Info(fmt.Sprintf("🎮 Party server starting on port %d", s.port))
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error(fmt.Sprintf("❌ Party server error: %v", err))
		}
	}()

	return nil
}

// Stop stops the party server
func (s *Server) Stop() error {
	s.logger.Info("Stopping party server...")

	// Cancel cleanup routine
	s.cancel()
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	// Close all client connections
	s.clientsMutex.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[*websocket.Conn]*ClientInfo)
	s.clientsMutex.Unlock()

	// Shutdown HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}

	return nil
}

// cleanupRoutine periodically cleans up inactive parties
func (s *Server) cleanupRoutine() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.registry.CleanupInactive()
			s.logger.Info("Cleaned up inactive parties")
		case <-s.ctx.Done():
			return
		}
	}
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error(fmt.Sprintf("❌ Failed to upgrade WebSocket connection from %s: %v", clientIP, err))
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusBadRequest)
		return
	}

	s.logger.Info(fmt.Sprintf("✅ New WebSocket connection established from %s", clientIP))

	// Handle client messages
	go s.handleClient(conn)
}

// handleClient handles messages from a single client
func (s *Server) handleClient(conn *websocket.Conn) {
	defer func() {
		s.removeClient(conn)
		conn.Close()
	}()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping routine
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for range pingTicker.C {
			if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}()

	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Error(fmt.Sprintf("WebSocket error: %v", err))
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			s.sendError(conn, ErrorCodeInvalidMessage, "Invalid message format")
			continue
		}

		s.handleMessage(conn, &msg)
	}
}

// handleMessage routes messages to appropriate handlers
func (s *Server) handleMessage(conn *websocket.Conn, msg *Message) {
	// Check rate limiting
	clientInfo := s.getClientInfo(conn)
	if clientInfo != nil {
		if !s.checkRateLimit(clientInfo.RemoteID) {
			s.sendError(conn, ErrorCodeRateLimit, "Rate limit exceeded")
			return
		}
	}

	switch msg.Type {
	case MessageTypeJoin:
		s.handleJoin(conn, msg)
	case MessageTypePosition:
		s.handlePosition(conn, msg)
	case MessageTypeLeave:
		s.handleLeave(conn, msg)
	default:
		s.sendError(conn, ErrorCodeInvalidMessage, "Unknown message type")
	}
}

// handleJoin handles join requests
func (s *Server) handleJoin(conn *websocket.Conn, msg *Message) {
	// Get client IP for logging
	clientIP := "unknown"
	if addr := conn.RemoteAddr(); addr != nil {
		clientIP = addr.String()
	}

	dataBytes, _ := json.Marshal(msg.Data)
	var joinMsg JoinMessage
	if err := json.Unmarshal(dataBytes, &joinMsg); err != nil {
		s.logger.Error(fmt.Sprintf("❌ Join attempt failed from %s: Invalid message format - %v", clientIP, err))
		s.sendError(conn, ErrorCodeInvalidMessage, "Invalid join message")
		return
	}

	// Log the join attempt with all details
	s.logger.Info(fmt.Sprintf("🔔 Join attempt: PartyCode=%s, RemoteID=%s, DisplayName='%s', IP=%s, Time=%s",
		joinMsg.PartyCode,
		joinMsg.RemoteID,
		joinMsg.DisplayName,
		clientIP,
		time.Now().Format("15:04:05"),
	))

	// Get party
	party, exists := s.registry.GetParty(joinMsg.PartyCode)
	if !exists {
		s.logger.Error(fmt.Sprintf("❌ Join failed: Party '%s' not found (requested by %s from %s)",
			joinMsg.PartyCode, joinMsg.DisplayName, clientIP))
		s.sendError(conn, ErrorCodeInvalidParty, "Party not found")
		return
	}

	// Add member to party
	if err := party.AddMember(joinMsg.RemoteID, joinMsg.DisplayName, conn); err != nil {
		if err.Error() == "party is full (max 5 members)" {
			s.logger.Error(fmt.Sprintf("❌ Join failed: Party '%s' is full - %s from %s denied",
				joinMsg.PartyCode, joinMsg.DisplayName, clientIP))
			s.sendError(conn, ErrorCodePartyFull, err.Error())
		} else {
			s.logger.Error(fmt.Sprintf("❌ Join failed: %s from %s - %v",
				joinMsg.DisplayName, clientIP, err))
			s.sendError(conn, ErrorCodeAlreadyInParty, err.Error())
		}
		return
	}

	// Register client
	s.clientsMutex.Lock()
	s.clients[conn] = &ClientInfo{
		RemoteID:    joinMsg.RemoteID,
		PartyCode:   joinMsg.PartyCode,
		DisplayName: joinMsg.DisplayName,
		Conn:        conn,
	}
	s.clientsMutex.Unlock()

	s.logger.Info(fmt.Sprintf("✅ SUCCESS: %s (RemoteID: %s) joined party %s from %s",
		joinMsg.DisplayName, joinMsg.RemoteID, joinMsg.PartyCode, clientIP))

	// Send welcome message to the joining client
	members := party.GetMembers()
	welcomeMsg := WelcomeMessage{
		PartyCode: joinMsg.PartyCode,
		IsHost:    party.IsHost(joinMsg.RemoteID),
		Members:   ConvertMembersToInfo(members),
		JoinedAt:  time.Now(),
	}
	s.sendMessage(conn, NewMessage(MessageTypeWelcome, welcomeMsg))

	// Notify other members
	member, _ := party.GetMember(joinMsg.RemoteID)
	memberJoinedMsg := MemberJoinedMessage{
		Member: ConvertMemberToInfo(member),
	}
	s.broadcastToParty(joinMsg.PartyCode, joinMsg.RemoteID, NewMessage(MessageTypeMemberJoined, memberJoinedMsg))
}

// handlePosition handles position updates
func (s *Server) handlePosition(conn *websocket.Conn, msg *Message) {
	clientInfo := s.getClientInfo(conn)
	if clientInfo == nil {
		s.sendError(conn, ErrorCodeNotInParty, "Not in a party")
		return
	}

	dataBytes, _ := json.Marshal(msg.Data)
	var posMsg PositionMessage
	if err := json.Unmarshal(dataBytes, &posMsg); err != nil {
		s.sendError(conn, ErrorCodeInvalidMessage, "Invalid position message")
		return
	}

	// Get party and update position
	party, exists := s.registry.GetParty(clientInfo.PartyCode)
	if !exists {
		s.sendError(conn, ErrorCodeInvalidParty, "Party not found")
		return
	}

	if err := party.UpdateMemberPosition(clientInfo.RemoteID, posMsg.Position, posMsg.Map); err != nil {
		s.sendError(conn, ErrorCodeInternalError, err.Error())
		return
	}

	// Broadcast position update to other members
	updateMsg := PositionUpdateMessage{
		RemoteID:    clientInfo.RemoteID,
		DisplayName: clientInfo.DisplayName,
		Map:         posMsg.Map,
		Position:    posMsg.Position,
		Timestamp:   time.Now(),
	}
	s.broadcastToParty(clientInfo.PartyCode, clientInfo.RemoteID, NewMessage(MessageTypePositionUpdate, updateMsg))
}

// handleLeave handles leave requests
func (s *Server) handleLeave(conn *websocket.Conn, msg *Message) {
	clientInfo := s.getClientInfo(conn)
	if clientInfo == nil {
		return
	}

	s.removeClientFromParty(clientInfo)
}

// removeClient removes a client from the server
func (s *Server) removeClient(conn *websocket.Conn) {
	clientInfo := s.getClientInfo(conn)
	if clientInfo != nil {
		s.removeClientFromParty(clientInfo)
	}

	s.clientsMutex.Lock()
	delete(s.clients, conn)
	s.clientsMutex.Unlock()
}

// removeClientFromParty removes a client from their party
func (s *Server) removeClientFromParty(clientInfo *ClientInfo) {
	party, exists := s.registry.GetParty(clientInfo.PartyCode)
	if !exists {
		return
	}

	party.RemoveMember(clientInfo.RemoteID)
	s.logger.Info(fmt.Sprintf("Member %s left party %s", clientInfo.DisplayName, clientInfo.PartyCode))

	// Notify other members
	leftMsg := MemberLeftMessage{
		RemoteID: clientInfo.RemoteID,
	}
	s.broadcastToParty(clientInfo.PartyCode, clientInfo.RemoteID, NewMessage(MessageTypeMemberLeft, leftMsg))

	// If party is empty, delete it
	if party.GetMemberCount() == 0 {
		s.registry.DeleteParty(clientInfo.PartyCode)
		s.logger.Info(fmt.Sprintf("Party %s deleted (no members)", clientInfo.PartyCode))
	}
}

// getClientInfo retrieves client info for a connection
func (s *Server) getClientInfo(conn *websocket.Conn) *ClientInfo {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return s.clients[conn]
}

// checkRateLimit checks if a client is within rate limits
func (s *Server) checkRateLimit(remoteID string) bool {
	s.rateLimiterMu.Lock()
	limiter, exists := s.rateLimiters[remoteID]
	if !exists {
		limiter = NewRateLimiter()
		s.rateLimiters[remoteID] = limiter
	}
	s.rateLimiterMu.Unlock()

	return limiter.Allow()
}

// sendMessage sends a message to a specific connection
func (s *Server) sendMessage(conn *websocket.Conn, msg *Message) {
	if err := conn.WriteJSON(msg); err != nil {
		s.logger.Error(fmt.Sprintf("Failed to send message: %v", err))
	}
}

// sendError sends an error message to a connection
func (s *Server) sendError(conn *websocket.Conn, code, message string) {
	s.sendMessage(conn, NewErrorMessage(code, message))
}

// broadcastToParty broadcasts a message to all members of a party except the sender
func (s *Server) broadcastToParty(partyCode, excludeRemoteID string, msg *Message) {
	s.clientsMutex.RLock()
	var targets []*websocket.Conn
	for conn, clientInfo := range s.clients {
		if clientInfo.PartyCode == partyCode && clientInfo.RemoteID != excludeRemoteID {
			targets = append(targets, conn)
		}
	}
	s.clientsMutex.RUnlock()

	for _, conn := range targets {
		go s.sendMessage(conn, msg)
	}
}

// CreateParty creates a new party (called by Wails bindings)
func (s *Server) CreateParty(hostRemoteID, displayName string) (string, error) {
	party, err := s.registry.CreateParty(hostRemoteID, displayName)
	if err != nil {
		return "", err
	}
	s.logger.Info(fmt.Sprintf("🎮 Party created with code: %s (Host: %s)", party.Code, displayName))
	return party.Code, nil
}

// GetPartyStatus returns the status of a party
func (s *Server) GetPartyStatus(partyCode string) (*Party, bool) {
	return s.registry.GetParty(partyCode)
}
