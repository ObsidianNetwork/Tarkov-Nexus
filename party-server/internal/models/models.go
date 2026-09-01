package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a connected user
type Client struct {
	ID          string          `json:"clientId"`
	RemoteID    string          `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string          `json:"displayName"`
	Conn        *websocket.Conn `json:"-"`
	PartyCode   string          `json:"partyCode,omitempty"`
	CurrentMap  string          `json:"currentMap,omitempty"`
	Position    *Position       `json:"position,omitempty"`
	LastUpdate  time.Time       `json:"lastUpdate,omitempty"`
	ConnectedAt time.Time       `json:"connectedAt"`
	IsTest      bool            `json:"isTest,omitempty"` // True for virtual test clients
	mu          sync.RWMutex
}

// Position represents player coordinates
type Position struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Rotation float64 `json:"rotation"`
}

// Party represents a party group
type Party struct {
	Code      string             `json:"code"`
	HostID    string             `json:"hostId"`
	Members   map[string]*Client `json:"-"`
	CreatedAt time.Time          `json:"createdAt"`
	mu        sync.RWMutex
}

// Friend represents a friend relationship
type Friend struct {
	ClientID       string    `json:"clientId"`
	FriendID       string    `json:"friendId"`
	FriendName     string    `json:"friendName"`
	AddedAt        time.Time `json:"addedAt"`
	LastPlayedWith time.Time `json:"lastPlayedWith,omitempty"`
}

// FriendStatus represents a friend with online status
type FriendStatus struct {
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
	Online      bool   `json:"online"`
	InParty     bool   `json:"inParty"`
	PartyCode   string `json:"partyCode,omitempty"`
}

// PartyInvite represents a pending party invitation
type PartyInvite struct {
	FromClientID   string    `json:"fromClientId"`
	FromName       string    `json:"fromDisplayName"`
	ToClientID     string    `json:"toClientId"`
	PartyCode      string    `json:"partyCode"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// Client methods
func (c *Client) SetParty(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PartyCode = code
}

func (c *Client) GetParty() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PartyCode
}

func (c *Client) UpdatePosition(pos *Position, mapName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Position = pos
	c.CurrentMap = mapName
	c.LastUpdate = time.Now()
}

func (c *Client) GetPosition() (*Position, string, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Position, c.CurrentMap, c.LastUpdate
}

// Party methods
func NewParty(code, hostID string) *Party {
	return &Party{
		Code:      code,
		HostID:    hostID,
		Members:   make(map[string]*Client),
		CreatedAt: time.Now(),
	}
}

func (p *Party) AddMember(client *Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.Members) >= 5 {
		return false
	}

	p.Members[client.ID] = client
	client.SetParty(p.Code)
	return true
}

func (p *Party) RemoveMember(clientID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.Members[clientID]; exists {
		client.SetParty("")
		delete(p.Members, clientID)
	}
}

func (p *Party) GetMembers() []*Client {
	p.mu.RLock()
	defer p.mu.RUnlock()

	members := make([]*Client, 0, len(p.Members))
	for _, m := range p.Members {
		members = append(members, m)
	}
	return members
}

func (p *Party) MemberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Members)
}

func (p *Party) IsEmpty() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Members) == 0
}

func (p *Party) GetHost() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.HostID
}
