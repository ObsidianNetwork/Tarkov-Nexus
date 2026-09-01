package models

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestParseMessage(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		wantType interface{}
		validate func(t *testing.T, msg interface{})
	}{
		{
			name:     "register",
			input:    `{"type":"register","clientId":"client-0001","displayName":"Alice","remoteId":"remote-1"}`,
			wantType: &RegisterMessage{},
			validate: func(t *testing.T, msg interface{}) {
				m := msg.(*RegisterMessage)
				if m.ClientID != "client-0001" || m.DisplayName != "Alice" || m.RemoteID != "remote-1" {
					t.Errorf("unexpected register fields: %+v", m)
				}
			},
		},
		{
			name:     "create_party",
			input:    `{"type":"create_party"}`,
			wantType: &CreatePartyMessage{},
		},
		{
			name:     "join_party",
			input:    `{"type":"join_party","partyCode":"ALPHA-1234"}`,
			wantType: &JoinPartyMessage{},
			validate: func(t *testing.T, msg interface{}) {
				if m := msg.(*JoinPartyMessage); m.PartyCode != "ALPHA-1234" {
					t.Errorf("unexpected party code: %s", m.PartyCode)
				}
			},
		},
		{
			name:     "leave_party",
			input:    `{"type":"leave_party"}`,
			wantType: &LeavePartyMessage{},
		},
		{
			name:     "position",
			input:    `{"type":"position","map":"Customs","x":1.5,"y":2.5,"z":3.5,"rotation":90}`,
			wantType: &PositionMessage{},
			validate: func(t *testing.T, msg interface{}) {
				m := msg.(*PositionMessage)
				if m.Map != "Customs" || m.X != 1.5 || m.Y != 2.5 || m.Z != 3.5 || m.Rotation != 90 {
					t.Errorf("unexpected position fields: %+v", m)
				}
			},
		},
		{
			name:     "add_friend",
			input:    `{"type":"add_friend","targetClientId":"client-0002"}`,
			wantType: &AddFriendMessage{},
		},
		{
			name:     "accept_friend_request",
			input:    `{"type":"accept_friend_request","targetClientId":"client-0002"}`,
			wantType: &AcceptFriendRequestMessage{},
		},
		{
			name:     "decline_friend_request",
			input:    `{"type":"decline_friend_request","targetClientId":"client-0002"}`,
			wantType: &DeclineFriendRequestMessage{},
		},
		{
			name:     "cancel_friend_request",
			input:    `{"type":"cancel_friend_request","targetClientId":"client-0002"}`,
			wantType: &CancelFriendRequestMessage{},
		},
		{
			name:     "get_pending_friend_requests",
			input:    `{"type":"get_pending_friend_requests"}`,
			wantType: &GetPendingFriendRequestsMessage{},
		},
		{
			name:     "remove_friend",
			input:    `{"type":"remove_friend","targetClientId":"client-0002"}`,
			wantType: &RemoveFriendMessage{},
		},
		{
			name:     "get_friends",
			input:    `{"type":"get_friends"}`,
			wantType: &GetFriendsMessage{},
		},
		{
			name:     "invite_friend",
			input:    `{"type":"invite_friend","targetClientId":"client-0002"}`,
			wantType: &InviteFriendMessage{},
		},
		{
			name:     "accept_invite",
			input:    `{"type":"accept_invite","partyCode":"ALPHA-1234"}`,
			wantType: &AcceptInviteMessage{},
		},
		{
			name:     "decline_invite",
			input:    `{"type":"decline_invite","partyCode":"ALPHA-1234"}`,
			wantType: &DeclineInviteMessage{},
		},
		{
			name:     "cancel_invite",
			input:    `{"type":"cancel_invite","targetClientId":"client-0002"}`,
			wantType: &CancelInviteMessage{},
		},
		{
			name:     "ping returns BaseMessage",
			input:    `{"type":"ping"}`,
			wantType: &BaseMessage{},
			validate: func(t *testing.T, msg interface{}) {
				if m := msg.(*BaseMessage); m.Type != MsgTypePing {
					t.Errorf("expected ping type, got %s", m.Type)
				}
			},
		},
		{
			name:     "unknown type returns BaseMessage",
			input:    `{"type":"some_future_type","extra":1}`,
			wantType: &BaseMessage{},
			validate: func(t *testing.T, msg interface{}) {
				if m := msg.(*BaseMessage); m.Type != "some_future_type" {
					t.Errorf("expected unknown type preserved, got %s", m.Type)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tc.input))
			if err != nil {
				t.Fatalf("ParseMessage failed: %v", err)
			}
			if got, want := reflect.TypeOf(msg), reflect.TypeOf(tc.wantType); got != want {
				t.Errorf("expected %v, got %v", want, got)
			}
			if tc.validate != nil {
				tc.validate(t, msg)
			}
		})
	}
}

func TestParseMessageInvalidJSON(t *testing.T) {
	for _, input := range []string{``, `not json`, `{"type":`, `{"type":123}`} {
		if _, err := ParseMessage([]byte(input)); err == nil {
			t.Errorf("expected error for input %q, got nil", input)
		}
	}
}

func TestParseMessageFieldMismatch(t *testing.T) {
	// Wrong JSON shape for a known type should surface the unmarshal error
	if _, err := ParseMessage([]byte(`{"type":"position","x":"not-a-number"}`)); err == nil {
		t.Error("expected unmarshal error for malformed position message")
	}
}

func newTestClient(id string) *Client {
	return &Client{
		ID:          id,
		DisplayName: "User_" + id,
		ConnectedAt: time.Now(),
	}
}

func TestPartyMemberCap(t *testing.T) {
	p := NewParty("ALPHA-1234", "client-0001")

	// Max 5 members
	for i := 1; i <= 5; i++ {
		c := newTestClient(fmt.Sprintf("client-%04d", i))
		if !p.AddMember(c) {
			t.Fatalf("AddMember %d should succeed", i)
		}
	}

	sixth := newTestClient("client-0006")
	if p.AddMember(sixth) {
		t.Error("AddMember should reject the 6th member")
	}
	if sixth.GetParty() != "" {
		t.Error("rejected member should not have a party code set")
	}
	if p.MemberCount() != 5 {
		t.Errorf("expected 5 members, got %d", p.MemberCount())
	}
}

func TestPartyLifecycle(t *testing.T) {
	p := NewParty("BRAVO-9999", "host-0001")
	if p.GetHost() != "host-0001" {
		t.Errorf("expected host-0001, got %s", p.GetHost())
	}
	if !p.IsEmpty() {
		t.Error("new party should be empty until host is added")
	}

	host := newTestClient("host-0001")
	p.AddMember(host)
	if host.GetParty() != "BRAVO-9999" {
		t.Errorf("host party code not set: %q", host.GetParty())
	}
	if len(p.GetMembers()) != 1 {
		t.Errorf("expected 1 member, got %d", len(p.GetMembers()))
	}

	p.RemoveMember(host.ID)
	if host.GetParty() != "" {
		t.Error("removed member should have party code cleared")
	}
	if !p.IsEmpty() {
		t.Error("party should be empty after removing last member")
	}

	// Removing a non-member should be a no-op
	p.RemoveMember("ghost-0000")
}

func TestClientPosition(t *testing.T) {
	c := newTestClient("client-0001")

	pos, mapName, _ := c.GetPosition()
	if pos != nil || mapName != "" {
		t.Error("new client should have no position")
	}

	before := time.Now()
	c.UpdatePosition(&Position{X: 10, Y: 20, Z: 30, Rotation: 45}, "Interchange")

	pos, mapName, lastUpdate := c.GetPosition()
	if pos == nil {
		t.Fatal("position should be set")
	}
	if *pos != (Position{X: 10, Y: 20, Z: 30, Rotation: 45}) {
		t.Errorf("unexpected position: %+v", pos)
	}
	if mapName != "Interchange" {
		t.Errorf("unexpected map: %s", mapName)
	}
	if lastUpdate.Before(before) {
		t.Error("LastUpdate should be refreshed")
	}
}
