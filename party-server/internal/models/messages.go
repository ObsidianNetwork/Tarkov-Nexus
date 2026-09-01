package models

import "encoding/json"

// Message types - Client to Server
const (
	MsgTypeRegister              = "register"
	MsgTypeCreateParty           = "create_party"
	MsgTypeJoinParty             = "join_party"
	MsgTypeLeaveParty            = "leave_party"
	MsgTypePosition              = "position"
	MsgTypeAddFriend             = "add_friend"              // Now sends a friend request
	MsgTypeAcceptFriendRequest   = "accept_friend_request"
	MsgTypeDeclineFriendRequest  = "decline_friend_request"
	MsgTypeCancelFriendRequest   = "cancel_friend_request"
	MsgTypeGetPendingFriendReqs  = "get_pending_friend_requests"
	MsgTypeRemoveFriend          = "remove_friend"
	MsgTypeGetFriends            = "get_friends"
	MsgTypeInviteFriend          = "invite_friend"
	MsgTypeAcceptInvite          = "accept_invite"
	MsgTypeDeclineInvite         = "decline_invite"
	MsgTypeCancelInvite          = "cancel_invite"
	MsgTypePing                  = "ping"
)

// Message types - Server to Client
const (
	MsgTypeRegistered              = "registered"
	MsgTypePartyCreated            = "party_created"
	MsgTypePartyJoined             = "party_joined"
	MsgTypeMemberJoined            = "member_joined"
	MsgTypeMemberLeft              = "member_left"
	MsgTypePositionUpdate          = "position_update"
	MsgTypeFriendsList             = "friends_list"
	MsgTypeFriendOnline            = "friend_online"
	MsgTypeFriendOffline           = "friend_offline"
	MsgTypeFriendRequest           = "friend_request"           // Incoming friend request
	MsgTypeFriendRequestSent       = "friend_request_sent"      // Confirmation request was sent
	MsgTypeFriendRequestAccepted   = "friend_request_accepted"  // Someone accepted your request
	MsgTypeFriendRequestDeclined   = "friend_request_declined"  // Someone declined your request
	MsgTypeFriendRequestCancelled  = "friend_request_cancelled" // Someone cancelled their request
	MsgTypeFriendRequestsList      = "friend_requests_list"     // List of pending requests
	MsgTypePartyInvite             = "party_invite"
	MsgTypeInviteAccepted          = "invite_accepted"
	MsgTypeInviteDeclined          = "invite_declined"
	MsgTypeInviteCancelled         = "invite_cancelled"
	MsgTypeError                   = "error"
	MsgTypePong                    = "pong"
	MsgTypePartyLeft               = "party_left"
	MsgTypeFriendAdded             = "friend_added"
	MsgTypeFriendRemoved           = "friend_removed"
)

// Error codes
const (
	ErrCodeInvalidMessage  = "INVALID_MESSAGE"
	ErrCodeInvalidParty    = "INVALID_PARTY"
	ErrCodePartyFull       = "PARTY_FULL"
	ErrCodeNotInParty      = "NOT_IN_PARTY"
	ErrCodeAlreadyInParty  = "ALREADY_IN_PARTY"
	ErrCodeNotRegistered   = "NOT_REGISTERED"
	ErrCodeFriendNotFound  = "FRIEND_NOT_FOUND"
	ErrCodeAlreadyFriends  = "ALREADY_FRIENDS"
	ErrCodeInviteNotFound  = "INVITE_NOT_FOUND"
	ErrCodeInternalError   = "INTERNAL_ERROR"
	ErrCodeRateLimit       = "RATE_LIMIT"
)

// BaseMessage is the base structure for all messages
type BaseMessage struct {
	Type string `json:"type"`
}

// Client to Server Messages

type RegisterMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	RemoteID    string `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string `json:"displayName"`
}

type CreatePartyMessage struct {
	Type string `json:"type"`
}

type JoinPartyMessage struct {
	Type      string `json:"type"`
	PartyCode string `json:"partyCode"`
}

type LeavePartyMessage struct {
	Type string `json:"type"`
}

type PositionMessage struct {
	Type     string  `json:"type"`
	Map      string  `json:"map"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	Rotation float64 `json:"rotation"`
}

type AddFriendMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"`
}

type AcceptFriendRequestMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"` // The person who sent the request
}

type DeclineFriendRequestMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"` // The person who sent the request
}

type CancelFriendRequestMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"` // The person you sent request to
}

type GetPendingFriendRequestsMessage struct {
	Type string `json:"type"`
}

type RemoveFriendMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"`
}

type GetFriendsMessage struct {
	Type string `json:"type"`
}

type InviteFriendMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"`
}

type AcceptInviteMessage struct {
	Type      string `json:"type"`
	PartyCode string `json:"partyCode"`
}

type DeclineInviteMessage struct {
	Type      string `json:"type"`
	PartyCode string `json:"partyCode"`
}

type CancelInviteMessage struct {
	Type           string `json:"type"`
	TargetClientID string `json:"targetClientId"`
}

// Server to Client Messages

type RegisteredMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"clientId"`
}

type PartyCreatedMessage struct {
	Type      string `json:"type"`
	PartyCode string `json:"partyCode"`
}

type PartyJoinedMessage struct {
	Type      string         `json:"type"`
	PartyCode string         `json:"partyCode"`
	Members   []MemberInfo   `json:"members"`
}

type MemberInfo struct {
	ClientID    string    `json:"clientId"`
	RemoteID    string    `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string    `json:"displayName"`
	CurrentMap  string    `json:"currentMap,omitempty"`
	Position    *Position `json:"position,omitempty"`
	IsHost      bool      `json:"isHost"`
}

type MemberJoinedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	RemoteID    string `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string `json:"displayName"`
}

type MemberLeftMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"clientId"`
}

type PositionUpdateMessage struct {
	Type        string   `json:"type"`
	ClientID    string   `json:"clientId"`
	RemoteID    string   `json:"remoteId,omitempty"` // tarkov.dev Remote ID for map markers
	DisplayName string   `json:"displayName"`
	Map         string   `json:"map"`
	Position    Position `json:"position"`
}

type FriendsListMessage struct {
	Type    string         `json:"type"`
	Friends []FriendStatus `json:"friends"`
}

type FriendOnlineMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type FriendOfflineMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"clientId"`
}

// Friend request messages (Server to Client)

type FriendRequestMessage struct {
	Type        string `json:"type"`
	FromClientID string `json:"fromClientId"`
	DisplayName string `json:"displayName"`
}

type FriendRequestSentMessage struct {
	Type        string `json:"type"`
	ToClientID  string `json:"toClientId"`
	DisplayName string `json:"displayName"`
}

type FriendRequestAcceptedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type FriendRequestDeclinedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type FriendRequestCancelledMessage struct {
	Type        string `json:"type"`
	FromClientID string `json:"fromClientId"`
}

type FriendRequestInfo struct {
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
	SentAt      string `json:"sentAt"`
}

type FriendRequestsListMessage struct {
	Type     string              `json:"type"`
	Incoming []FriendRequestInfo `json:"incoming"` // Requests sent TO you
	Outgoing []FriendRequestInfo `json:"outgoing"` // Requests you sent
}

type PartyInviteMessage struct {
	Type        string `json:"type"`
	FromClientID string `json:"fromClientId"`
	FromName    string `json:"fromDisplayName"`
	PartyCode   string `json:"partyCode"`
}

type InviteAcceptedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type InviteDeclinedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type InviteCancelledMessage struct {
	Type         string `json:"type"`
	FromClientID string `json:"fromClientId"`
	PartyCode    string `json:"partyCode"`
}

type PartyLeftMessage struct {
	Type string `json:"type"`
}

type FriendAddedMessage struct {
	Type        string `json:"type"`
	ClientID    string `json:"clientId"`
	DisplayName string `json:"displayName"`
}

type FriendRemovedMessage struct {
	Type     string `json:"type"`
	ClientID string `json:"clientId"`
}

type ErrorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PongMessage struct {
	Type string `json:"type"`
}

// ParseMessage parses a raw JSON message into the appropriate type
func ParseMessage(data []byte) (interface{}, error) {
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	var msg interface{}
	switch base.Type {
	case MsgTypeRegister:
		msg = &RegisterMessage{}
	case MsgTypeCreateParty:
		msg = &CreatePartyMessage{}
	case MsgTypeJoinParty:
		msg = &JoinPartyMessage{}
	case MsgTypeLeaveParty:
		msg = &LeavePartyMessage{}
	case MsgTypePosition:
		msg = &PositionMessage{}
	case MsgTypeAddFriend:
		msg = &AddFriendMessage{}
	case MsgTypeAcceptFriendRequest:
		msg = &AcceptFriendRequestMessage{}
	case MsgTypeDeclineFriendRequest:
		msg = &DeclineFriendRequestMessage{}
	case MsgTypeCancelFriendRequest:
		msg = &CancelFriendRequestMessage{}
	case MsgTypeGetPendingFriendReqs:
		msg = &GetPendingFriendRequestsMessage{}
	case MsgTypeRemoveFriend:
		msg = &RemoveFriendMessage{}
	case MsgTypeGetFriends:
		msg = &GetFriendsMessage{}
	case MsgTypeInviteFriend:
		msg = &InviteFriendMessage{}
	case MsgTypeAcceptInvite:
		msg = &AcceptInviteMessage{}
	case MsgTypeDeclineInvite:
		msg = &DeclineInviteMessage{}
	case MsgTypeCancelInvite:
		msg = &CancelInviteMessage{}
	case MsgTypePing:
		return &BaseMessage{Type: MsgTypePing}, nil
	default:
		return &base, nil
	}

	if err := json.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
