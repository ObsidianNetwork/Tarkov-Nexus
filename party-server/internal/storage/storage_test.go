//go:build cgo

// This file requires CGO because go-sqlite3 is a cgo package. When built
// with CGO_ENABLED=0 the driver is a stub that errors at runtime, so these
// tests are compiled only when a C toolchain is available (CI, Docker).

package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStorage creates a Storage backed by a temporary directory.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// registerPair registers two clients and fails the test on error.
func registerPair(t *testing.T, s *Storage, idA, idB string) {
	t.Helper()
	if err := s.RegisterClient(idA, "Name_"+idA); err != nil {
		t.Fatalf("RegisterClient(%s) failed: %v", idA, err)
	}
	if err := s.RegisterClient(idB, "Name_"+idB); err != nil {
		t.Fatalf("RegisterClient(%s) failed: %v", idB, err)
	}
}

func TestNewStorageCreatesDirAndDB(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(filepath.Join(dir, "party.db")); err != nil {
		t.Errorf("expected party.db to exist: %v", err)
	}

	// Re-opening the same DB must succeed (schema is idempotent)
	s2, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("re-opening storage failed: %v", err)
	}
	s2.Close()
}

func TestRegisterClientUpsert(t *testing.T) {
	s := newTestStorage(t)

	if err := s.RegisterClient("client-0001", "Alice"); err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	name, exists, err := s.GetClient("client-0001")
	if err != nil || !exists {
		t.Fatalf("GetClient failed: exists=%v err=%v", exists, err)
	}
	if name != "Alice" {
		t.Errorf("expected Alice, got %s", name)
	}

	// Re-register with a new display name — should update, not error
	if err := s.RegisterClient("client-0001", "AliceV2"); err != nil {
		t.Fatalf("re-register failed: %v", err)
	}
	name, exists, err = s.GetClient("client-0001")
	if err != nil || !exists {
		t.Fatalf("GetClient after re-register failed: exists=%v err=%v", exists, err)
	}
	if name != "AliceV2" {
		t.Errorf("expected updated name AliceV2, got %s", name)
	}
}

func TestGetClientNotFound(t *testing.T) {
	s := newTestStorage(t)

	name, exists, err := s.GetClient("ghost-0000")
	if err != nil {
		t.Fatalf("GetClient should not error for missing client: %v", err)
	}
	if exists || name != "" {
		t.Errorf("expected missing client, got exists=%v name=%q", exists, name)
	}
}

func TestUpdateLastSeen(t *testing.T) {
	s := newTestStorage(t)
	if err := s.RegisterClient("client-0001", "Alice"); err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	// Backdate last_seen, then verify UpdateLastSeen actually advances it
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec(`UPDATE clients SET last_seen = ? WHERE client_id = ?`, old, "client-0001"); err != nil {
		t.Fatalf("failed to backdate last_seen: %v", err)
	}

	if err := s.UpdateLastSeen("client-0001"); err != nil {
		t.Fatalf("UpdateLastSeen failed: %v", err)
	}

	var lastSeen time.Time
	if err := s.db.QueryRow(`SELECT last_seen FROM clients WHERE client_id = ?`, "client-0001").Scan(&lastSeen); err != nil {
		t.Fatalf("failed to read last_seen: %v", err)
	}
	if !lastSeen.After(old) {
		t.Errorf("last_seen was not advanced: still %v", lastSeen)
	}
}

func TestAddFriendBidirectional(t *testing.T) {
	s := newTestStorage(t)
	registerPair(t, s, "client-0001", "client-0002")

	if err := s.AddFriend("client-0001", "client-0002", "Name_client-0002"); err != nil {
		t.Fatalf("AddFriend failed: %v", err)
	}

	for _, pair := range [][2]string{{"client-0001", "client-0002"}, {"client-0002", "client-0001"}} {
		ok, err := s.AreFriends(pair[0], pair[1])
		if err != nil || !ok {
			t.Errorf("AreFriends(%s, %s) = %v, %v; want true", pair[0], pair[1], ok, err)
		}
	}

	// Both directions should resolve the *other* user's display name
	friendsA, err := s.GetFriends("client-0001")
	if err != nil || len(friendsA) != 1 {
		t.Fatalf("GetFriends(A) failed: %v len=%d", err, len(friendsA))
	}
	if friendsA[0].FriendID != "client-0002" || friendsA[0].FriendName != "Name_client-0002" {
		t.Errorf("unexpected friend record: %+v", friendsA[0])
	}

	friendsB, err := s.GetFriends("client-0002")
	if err != nil || len(friendsB) != 1 {
		t.Fatalf("GetFriends(B) failed: %v len=%d", err, len(friendsB))
	}
	if friendsB[0].FriendName != "Name_client-0001" {
		t.Errorf("expected reverse name Name_client-0001, got %s", friendsB[0].FriendName)
	}

	// Re-adding is idempotent (upsert) — no duplicate rows
	if err := s.AddFriend("client-0001", "client-0002", "Name_client-0002"); err != nil {
		t.Fatalf("re-AddFriend failed: %v", err)
	}
	friendsAfter, err := s.GetFriends("client-0001")
	if err != nil {
		t.Fatalf("GetFriends after re-add failed: %v", err)
	}
	if len(friendsAfter) != 1 {
		t.Errorf("expected still 1 friend after duplicate add, got %d", len(friendsAfter))
	}
}

func TestAddFriendUnknownClient(t *testing.T) {
	s := newTestStorage(t)
	if err := s.RegisterClient("client-0002", "Bob"); err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	// client-0001 was never registered — display name lookup fails
	if err := s.AddFriend("client-0001", "client-0002", "Bob"); err == nil {
		t.Error("AddFriend should fail when the initiating client is not registered")
	}
}

func TestRemoveFriend(t *testing.T) {
	s := newTestStorage(t)
	registerPair(t, s, "client-0001", "client-0002")

	if err := s.AddFriend("client-0001", "client-0002", "Name_client-0002"); err != nil {
		t.Fatalf("AddFriend failed: %v", err)
	}
	if err := s.RemoveFriend("client-0001", "client-0002"); err != nil {
		t.Fatalf("RemoveFriend failed: %v", err)
	}

	for _, pair := range [][2]string{{"client-0001", "client-0002"}, {"client-0002", "client-0001"}} {
		ok, err := s.AreFriends(pair[0], pair[1])
		if err != nil || ok {
			t.Errorf("AreFriends(%s, %s) = %v, %v; want false", pair[0], pair[1], ok, err)
		}
	}
}

func TestGetFriendsOrderingByLastPlayed(t *testing.T) {
	s := newTestStorage(t)
	registerPair(t, s, "client-0001", "client-0002")
	if err := s.RegisterClient("client-0003", "Name_client-0003"); err != nil {
		t.Fatalf("RegisterClient failed: %v", err)
	}

	if err := s.AddFriend("client-0001", "client-0002", "Name_client-0002"); err != nil {
		t.Fatalf("AddFriend failed: %v", err)
	}
	if err := s.AddFriend("client-0001", "client-0003", "Name_client-0003"); err != nil {
		t.Fatalf("AddFriend failed: %v", err)
	}

	if err := s.UpdateLastPlayedWith("client-0001", "client-0003"); err != nil {
		t.Fatalf("UpdateLastPlayedWith failed: %v", err)
	}

	friends, err := s.GetFriends("client-0001")
	if err != nil || len(friends) != 2 {
		t.Fatalf("GetFriends failed: %v len=%d", err, len(friends))
	}

	// Most recently played-with sorts first (NULLS LAST for the other)
	if friends[0].FriendID != "client-0003" {
		t.Errorf("expected client-0003 first, got %s", friends[0].FriendID)
	}
	if friends[0].LastPlayedWith == nil {
		t.Error("expected LastPlayedWith to be set for client-0003")
	}
	if friends[1].LastPlayedWith != nil {
		t.Error("expected LastPlayedWith to be nil for client-0002")
	}
}

func TestFriendRequestLifecycle(t *testing.T) {
	s := newTestStorage(t)
	registerPair(t, s, "client-0001", "client-0002")

	exists, err := s.GetFriendRequest("client-0001", "client-0002")
	if err != nil || exists {
		t.Fatalf("no request should exist yet: exists=%v err=%v", exists, err)
	}

	if err := s.CreateFriendRequest("client-0001", "client-0002", "Name_client-0001", "Name_client-0002"); err != nil {
		t.Fatalf("CreateFriendRequest failed: %v", err)
	}

	// Duplicate create is a no-op (ON CONFLICT DO NOTHING)
	if err := s.CreateFriendRequest("client-0001", "client-0002", "Name_client-0001", "Name_client-0002"); err != nil {
		t.Fatalf("duplicate CreateFriendRequest failed: %v", err)
	}

	exists, err = s.GetFriendRequest("client-0001", "client-0002")
	if err != nil || !exists {
		t.Fatalf("request should exist: exists=%v err=%v", exists, err)
	}

	incoming, err := s.GetIncomingFriendRequests("client-0002")
	if err != nil || len(incoming) != 1 {
		t.Fatalf("GetIncomingFriendRequests failed: %v len=%d", err, len(incoming))
	}
	if incoming[0].ClientID != "client-0001" || incoming[0].DisplayName != "Name_client-0001" {
		t.Errorf("unexpected incoming request: %+v", incoming[0])
	}

	outgoing, err := s.GetOutgoingFriendRequests("client-0001")
	if err != nil || len(outgoing) != 1 {
		t.Fatalf("GetOutgoingFriendRequests failed: %v len=%d", err, len(outgoing))
	}
	if outgoing[0].ClientID != "client-0002" || outgoing[0].DisplayName != "Name_client-0002" {
		t.Errorf("unexpected outgoing request: %+v", outgoing[0])
	}

	// Reverse direction should not exist
	exists, err = s.GetFriendRequest("client-0002", "client-0001")
	if err != nil {
		t.Fatalf("GetFriendRequest failed: %v", err)
	}
	if exists {
		t.Error("reverse request should not exist")
	}

	if err := s.DeleteFriendRequest("client-0001", "client-0002"); err != nil {
		t.Fatalf("DeleteFriendRequest failed: %v", err)
	}
	exists, err = s.GetFriendRequest("client-0001", "client-0002")
	if err != nil {
		t.Fatalf("GetFriendRequest after delete failed: %v", err)
	}
	if exists {
		t.Error("request should be deleted")
	}
}
