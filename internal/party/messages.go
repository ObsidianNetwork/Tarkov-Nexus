package party

import "time"

// MessageType represents the type of WebSocket message
type MessageType string

const (
	// Client → Server messages
	MessageTypeJoin     MessageType = "join"
	MessageTypePosition MessageType = "position"
	MessageTypeLeave    MessageType = "leave"

	// Server → Client messages
	MessageTypeWelcome        MessageType = "welcome"
	MessageTypeMemberJoined   MessageType = "member_joined"
	MessageTypeMemberLeft     MessageType = "member_left"
	MessageTypePositionUpdate MessageType = "position_update"
	MessageTypeError          MessageType = "error"
)

// Message is the base WebSocket message structure
type Message struct {
	Type MessageType `json:"type"`
	Data interface{} `json:"data"`
}

// JoinMessage is sent when a client wants to join a party
type JoinMessage struct {
	PartyCode   string `json:"partyCode"`
	RemoteID    string `json:"remoteId"`
	DisplayName string `json:"displayName"`
}

// PositionMessage is sent when a client updates their position
type PositionMessage struct {
	RemoteID string    `json:"remoteId"`
	Map      string    `json:"map"`
	Position *Position `json:"position"`
}

// LeaveMessage is sent when a client leaves the party
type LeaveMessage struct {
	RemoteID string `json:"remoteId"`
}

// WelcomeMessage is sent to a client upon successful join
type WelcomeMessage struct {
	PartyCode   string         `json:"partyCode"`
	IsHost      bool           `json:"isHost"`
	Members     []MemberInfo   `json:"members"`
	JoinedAt    time.Time      `json:"joinedAt"`
}

// MemberJoinedMessage is sent when a new member joins
type MemberJoinedMessage struct {
	Member MemberInfo `json:"member"`
}

// MemberLeftMessage is sent when a member leaves
type MemberLeftMessage struct {
	RemoteID string `json:"remoteId"`
}

// PositionUpdateMessage is sent when a member updates their position
type PositionUpdateMessage struct {
	RemoteID    string    `json:"remoteId"`
	DisplayName string    `json:"displayName"`
	Map         string    `json:"map"`
	Position    *Position `json:"position"`
	Timestamp   time.Time `json:"timestamp"`
}

// ErrorMessage is sent when an error occurs
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MemberInfo represents member information for the frontend
type MemberInfo struct {
	RemoteID     string    `json:"remoteId"`
	DisplayName  string    `json:"displayName"`
	Connected    bool      `json:"connected"`
	CurrentMap   string    `json:"currentMap"`
	LastPosition *Position `json:"lastPosition,omitempty"`
	LastUpdate   time.Time `json:"lastUpdate"`
}

// Error codes for ErrorMessage
const (
	ErrorCodeInvalidParty     = "INVALID_PARTY"
	ErrorCodePartyFull        = "PARTY_FULL"
	ErrorCodeAlreadyInParty   = "ALREADY_IN_PARTY"
	ErrorCodeNotInParty       = "NOT_IN_PARTY"
	ErrorCodeInvalidMessage   = "INVALID_MESSAGE"
	ErrorCodeRateLimit        = "RATE_LIMIT"
	ErrorCodeInternalError    = "INTERNAL_ERROR"
)

// NewMessage creates a new message with the given type and data
func NewMessage(msgType MessageType, data interface{}) *Message {
	return &Message{
		Type: msgType,
		Data: data,
	}
}

// NewErrorMessage creates a new error message
func NewErrorMessage(code, message string) *Message {
	return &Message{
		Type: MessageTypeError,
		Data: ErrorMessage{
			Code:    code,
			Message: message,
		},
	}
}

// ConvertMemberToInfo converts a Member to MemberInfo (for JSON serialization)
func ConvertMemberToInfo(m *Member) MemberInfo {
	return MemberInfo{
		RemoteID:     m.RemoteID,
		DisplayName:  m.DisplayName,
		Connected:    m.Connected,
		CurrentMap:   m.CurrentMap,
		LastPosition: m.LastPosition,
		LastUpdate:   m.LastUpdate,
	}
}

// ConvertMembersToInfo converts a slice of Members to MemberInfo
func ConvertMembersToInfo(members []Member) []MemberInfo {
	infos := make([]MemberInfo, len(members))
	for i, m := range members {
		infos[i] = MemberInfo{
			RemoteID:     m.RemoteID,
			DisplayName:  m.DisplayName,
			Connected:    m.Connected,
			CurrentMap:   m.CurrentMap,
			LastPosition: m.LastPosition,
			LastUpdate:   m.LastUpdate,
		}
	}
	return infos
}
