package party

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Party represents a group of players sharing position data
type Party struct {
	ID           string
	Code         string
	Host         string // RemoteID of the party host
	Members      map[string]*Member
	CreatedAt    time.Time
	LastActivity time.Time
	mu           sync.RWMutex
}

// Member represents a party member
type Member struct {
	RemoteID     string
	DisplayName  string
	Connected    bool
	CurrentMap   string
	LastPosition *Position
	LastUpdate   time.Time
	Connection   interface{} // Will hold *websocket.Conn
}

// Position represents player position data
type Position struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Rotation float64 `json:"rotation"`
}

// PartyRegistry manages all active parties
type PartyRegistry struct {
	parties map[string]*Party // map[code]*Party
	mu      sync.RWMutex
}

// NewPartyRegistry creates a new party registry
func NewPartyRegistry() *PartyRegistry {
	return &PartyRegistry{
		parties: make(map[string]*Party),
	}
}

// CreateParty creates a new party with a unique code
func (r *PartyRegistry) CreateParty(hostRemoteID, displayName string) (*Party, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	code := generatePartyCode()

	// Ensure code is unique
	for r.parties[code] != nil {
		code = generatePartyCode()
	}

	party := &Party{
		ID:           generateID(),
		Code:         code,
		Host:         hostRemoteID,
		Members:      make(map[string]*Member),
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	// Don't auto-add host - they will join via WebSocket like other members
	// This allows the host to send/receive position updates properly

	r.parties[code] = party
	return party, nil
}

// GetParty retrieves a party by code
func (r *PartyRegistry) GetParty(code string) (*Party, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	party, exists := r.parties[code]
	return party, exists
}

// DeleteParty removes a party from the registry
func (r *PartyRegistry) DeleteParty(code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.parties, code)
}

// CleanupInactive removes parties with no activity for 24 hours
func (r *PartyRegistry) CleanupInactive() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	for code, party := range r.parties {
		party.mu.RLock()
		lastActivity := party.LastActivity
		party.mu.RUnlock()

		if lastActivity.Before(cutoff) {
			delete(r.parties, code)
		}
	}
}

// AddMember adds a member to the party
func (p *Party) AddMember(remoteID, displayName string, conn interface{}) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check party size limit (max 5 members)
	if len(p.Members) >= 5 {
		return fmt.Errorf("party is full (max 5 members)")
	}

	// Check if member already exists
	if _, exists := p.Members[remoteID]; exists {
		return fmt.Errorf("member already in party")
	}

	member := &Member{
		RemoteID:    remoteID,
		DisplayName: displayName,
		Connected:   true,
		Connection:  conn,
		LastUpdate:  time.Now(),
	}

	p.Members[remoteID] = member
	p.LastActivity = time.Now()

	return nil
}

// RemoveMember removes a member from the party
func (p *Party) RemoveMember(remoteID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.Members, remoteID)
	p.LastActivity = time.Now()
}

// GetMember retrieves a member by remoteID
func (p *Party) GetMember(remoteID string) (*Member, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	member, exists := p.Members[remoteID]
	return member, exists
}

// GetMembers returns a copy of all members (thread-safe)
func (p *Party) GetMembers() []Member {
	p.mu.RLock()
	defer p.mu.RUnlock()

	members := make([]Member, 0, len(p.Members))
	for _, m := range p.Members {
		// Create a copy to avoid race conditions
		memberCopy := Member{
			RemoteID:    m.RemoteID,
			DisplayName: m.DisplayName,
			Connected:   m.Connected,
			CurrentMap:  m.CurrentMap,
			LastUpdate:  m.LastUpdate,
		}
		if m.LastPosition != nil {
			pos := *m.LastPosition
			memberCopy.LastPosition = &pos
		}
		members = append(members, memberCopy)
	}
	return members
}

// UpdateMemberPosition updates a member's position
func (p *Party) UpdateMemberPosition(remoteID string, position *Position, mapName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	member, exists := p.Members[remoteID]
	if !exists {
		return fmt.Errorf("member not found")
	}

	member.LastPosition = position
	member.CurrentMap = mapName
	member.LastUpdate = time.Now()
	p.LastActivity = time.Now()

	return nil
}

// SetMemberConnected updates a member's connection status
func (p *Party) SetMemberConnected(remoteID string, connected bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if member, exists := p.Members[remoteID]; exists {
		member.Connected = connected
		if !connected {
			member.Connection = nil
		}
	}
}

// IsHost checks if the given remoteID is the party host
func (p *Party) IsHost(remoteID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Host == remoteID
}

// GetMemberCount returns the number of members in the party
func (p *Party) GetMemberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Members)
}

// generatePartyCode generates a random party code (e.g., "DELTA-7829")
func generatePartyCode() string {
	// Generate 2 random bytes for suffix
	bytes := make([]byte, 2)
	rand.Read(bytes)
	num := int(bytes[0])<<8 | int(bytes[1])

	// Use NATO phonetic alphabet prefixes
	prefixes := []string{
		"ALPHA", "BRAVO", "CHARLIE", "DELTA", "ECHO",
		"FOXTROT", "GOLF", "HOTEL", "INDIA", "JULIET",
		"KILO", "LIMA", "MIKE", "NOVEMBER", "OSCAR",
		"PAPA", "QUEBEC", "ROMEO", "SIERRA", "TANGO",
	}

	prefix := prefixes[num%len(prefixes)]
	return fmt.Sprintf("%s-%04d", prefix, num%10000)
}

// generateID generates a random unique ID
func generateID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
