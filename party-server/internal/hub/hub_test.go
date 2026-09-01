//go:build cgo

// This file requires CGO: the hub is coupled to *storage.Storage, which
// uses go-sqlite3. Tests are compiled only when a C toolchain is available.

package hub

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/party-server/internal/models"
	"tarkov-screenshot-analyzer/party-server/internal/storage"
)

// newTestHub creates a Hub backed by a temporary SQLite database.
func newTestHub(t *testing.T) *Hub {
	t.Helper()
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	h := NewHub(store)
	t.Cleanup(func() {
		h.Close() // stop the cleanup goroutine
		store.Close()
	})
	return h
}

// mustCreateTestClient creates a test client with an explicit ID (>= 8 chars,
// as the hub logs ID prefixes) and fails the test on error.
func mustCreateTestClient(t *testing.T, h *Hub, name, id string) *models.Client {
	t.Helper()
	c, err := h.CreateTestClient(name, id, "")
	if err != nil {
		t.Fatalf("CreateTestClient(%s) failed: %v", id, err)
	}
	return c
}

// TestHubCloseIdempotent verifies concurrent and repeated Close calls do
// not panic (sync.Once guards the channel close). Run with -race in CI.
func TestHubCloseIdempotent(t *testing.T) {
	h := newTestHub(t) // cleanup calls Close once more

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Close()
		}()
	}
	wg.Wait()
}

func TestGeneratePartyCode(t *testing.T) {
	h := newTestHub(t)
	pattern := regexp.MustCompile(`^[A-Z]+-\d{4}$`)

	nato := make(map[string]bool)
	for _, w := range natoAlphabet {
		nato[w] = true
	}

	for i := 0; i < 100; i++ {
		code := h.generatePartyCode()
		if !pattern.MatchString(code) {
			t.Fatalf("code %q does not match WORD-NNNN", code)
		}
		word := strings.SplitN(code, "-", 2)[0]
		if !nato[word] {
			t.Errorf("code %q uses non-NATO word %q", code, word)
		}
	}
}

func TestCreateTestClient(t *testing.T) {
	h := newTestHub(t)

	// Explicit values
	c := mustCreateTestClient(t, h, "Alice", "client-0001")
	if c.DisplayName != "Alice" || c.ID != "client-0001" || !c.IsTest {
		t.Errorf("unexpected client: %+v", c)
	}
	if c.RemoteID == "" {
		t.Error("expected a default remote ID")
	}

	// Defaults are generated when inputs are empty
	c2, err := h.CreateTestClient("", "", "")
	if err != nil {
		t.Fatalf("CreateTestClient with defaults failed: %v", err)
	}
	if !strings.HasPrefix(c2.DisplayName, "TestUser_") {
		t.Errorf("expected generated display name, got %q", c2.DisplayName)
	}

	// Duplicate ID is rejected
	if _, err := h.CreateTestClient("Alice2", "client-0001", ""); err == nil {
		t.Error("expected duplicate client ID to be rejected")
	}
}

func TestCreateTestParty(t *testing.T) {
	h := newTestHub(t)

	// Unknown host
	if _, err := h.CreateTestParty("ghost-0000", nil); err == nil {
		t.Error("expected error for unknown host")
	}

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	member := mustCreateTestClient(t, h, "Member", "member-001")

	code, err := h.CreateTestParty(host.ID, []string{member.ID, "ghost-0000"})
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}
	if host.GetParty() != code || member.GetParty() != code {
		t.Errorf("members not attached to party %s", code)
	}

	// Host already in a party
	if _, err := h.CreateTestParty(host.ID, nil); err == nil {
		t.Error("expected error when host is already in a party")
	}

	parties := h.GetAllParties()
	if len(parties) != 1 || parties[0].MemberCount != 2 {
		t.Errorf("unexpected parties: %+v", parties)
	}
}

func TestJoinTestPartyFull(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Fill to capacity (5 total)
	ids := make([]string, 0, 4)
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("member-%03d", i)
		mustCreateTestClient(t, h, "Member"+id, id)
		ids = append(ids, id)
		if err := h.JoinTestParty(id, code); err != nil {
			t.Fatalf("JoinTestParty(%s) failed: %v", id, err)
		}
	}

	// 6th member rejected
	late := mustCreateTestClient(t, h, "Late", "member-099")
	if err := h.JoinTestParty(late.ID, code); err == nil {
		t.Error("expected party-full error for 6th member")
	}

	// Unknown party / unknown client / double join
	if err := h.JoinTestParty(late.ID, "NOSUCH-000"); err == nil {
		t.Error("expected error for unknown party")
	}
	if err := h.JoinTestParty("ghost-0000", code); err == nil {
		t.Error("expected error for unknown client")
	}
	if err := h.JoinTestParty(ids[0], code); err == nil {
		t.Error("expected error for client already in a party")
	}
}

func TestLeaveTestPartyCleansUpEmpty(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	if _, err := h.CreateTestParty(host.ID, nil); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Not in a party / unknown client error paths
	if err := h.LeaveTestParty("ghost-0000"); err == nil {
		t.Error("expected error for unknown client")
	}
	loner := mustCreateTestClient(t, h, "Loner", "loner-0001")
	if err := h.LeaveTestParty(loner.ID); err == nil {
		t.Error("expected error for client not in a party")
	}

	if err := h.LeaveTestParty(host.ID); err != nil {
		t.Fatalf("LeaveTestParty failed: %v", err)
	}
	if host.GetParty() != "" {
		t.Error("host should no longer be in a party")
	}
	if got := h.GetStats()["parties"].(int); got != 0 {
		t.Errorf("empty party should be deleted, got %d parties", got)
	}
}

func TestDeleteParty(t *testing.T) {
	h := newTestHub(t)

	if err := h.DeleteParty("NOSUCH-000"); err == nil {
		t.Error("expected error for unknown party")
	}

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	if err := h.DeleteParty(code); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}
	if host.GetParty() != "" {
		t.Error("member party codes should be cleared after delete")
	}
	if got := h.GetStats()["parties"].(int); got != 0 {
		t.Errorf("expected 0 parties, got %d", got)
	}
}

func TestGetStats(t *testing.T) {
	h := newTestHub(t)

	stats := h.GetStats()
	if stats["clients"].(int) != 0 || stats["parties"].(int) != 0 || stats["testClients"].(int) != 0 {
		t.Errorf("expected zeroed stats, got %+v", stats)
	}

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	if _, err := h.CreateTestParty(host.ID, nil); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}
	// Inject a non-test client directly (white-box)
	h.mu.Lock()
	h.clients["real-client"] = &models.Client{ID: "real-client", DisplayName: "Real", ConnectedAt: time.Now()}
	h.mu.Unlock()

	stats = h.GetStats()
	if stats["clients"].(int) != 2 {
		t.Errorf("expected 2 clients, got %v", stats["clients"])
	}
	if stats["testClients"].(int) != 1 {
		t.Errorf("expected 1 test client, got %v", stats["testClients"])
	}
	if stats["parties"].(int) != 1 {
		t.Errorf("expected 1 party, got %v", stats["parties"])
	}
}

func TestGetAllClientsSortedByName(t *testing.T) {
	h := newTestHub(t)
	mustCreateTestClient(t, h, "charlie", "client-0003")
	mustCreateTestClient(t, h, "Alice", "client-0001")
	mustCreateTestClient(t, h, "bob", "client-0002")

	clients := h.GetAllClients()
	if len(clients) != 3 {
		t.Fatalf("expected 3 clients, got %d", len(clients))
	}
	want := []string{"Alice", "bob", "charlie"} // case-insensitive sort
	for i, name := range want {
		if clients[i].DisplayName != name {
			t.Errorf("position %d: expected %s, got %s", i, name, clients[i].DisplayName)
		}
	}
}

func TestRemoveTestClient(t *testing.T) {
	h := newTestHub(t)

	if err := h.RemoveTestClient("ghost-0000"); err == nil {
		t.Error("expected error for unknown client")
	}

	// Non-test clients are protected
	h.mu.Lock()
	h.clients["real-client"] = &models.Client{ID: "real-client", DisplayName: "Real", ConnectedAt: time.Now()}
	h.mu.Unlock()
	if err := h.RemoveTestClient("real-client"); err == nil {
		t.Error("expected refusal to remove a non-test client")
	}

	// Test client in a party: removal also cleans up the empty party
	host := mustCreateTestClient(t, h, "Host", "host-00001")
	if _, err := h.CreateTestParty(host.ID, nil); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}
	if err := h.RemoveTestClient(host.ID); err != nil {
		t.Fatalf("RemoveTestClient failed: %v", err)
	}
	stats := h.GetStats()
	if stats["clients"].(int) != 1 || stats["parties"].(int) != 0 {
		t.Errorf("unexpected stats after removal: %+v", stats)
	}
}

func TestCleanupTestClients(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	mustCreateTestClient(t, h, "Second", "client-0002")
	if _, err := h.CreateTestParty(host.ID, []string{"client-0002"}); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	h.mu.Lock()
	h.clients["real-client"] = &models.Client{ID: "real-client", DisplayName: "Real", ConnectedAt: time.Now()}
	h.mu.Unlock()

	removed := h.CleanupTestClients()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	stats := h.GetStats()
	if stats["clients"].(int) != 1 || stats["testClients"].(int) != 0 || stats["parties"].(int) != 0 {
		t.Errorf("unexpected stats after cleanup: %+v", stats)
	}
}

func TestFindClientByPrefix(t *testing.T) {
	h := newTestHub(t)
	mustCreateTestClient(t, h, "Alice", "abcdef00-client")

	if got := h.FindClientByPrefix("ABCDEF"); got != "abcdef00-client" {
		t.Errorf("expected case-insensitive prefix match, got %q", got)
	}
	if got := h.FindClientByPrefix("zzzz"); got != "" {
		t.Errorf("expected no match, got %q", got)
	}
}

func TestSendTestPosition(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")

	// Not in a party
	if err := h.SendTestPosition(host.ID, "Customs", 1, 2, 3, 0); err == nil {
		t.Error("expected error when client is not in a party")
	}

	if _, err := h.CreateTestParty(host.ID, nil); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}
	if err := h.SendTestPosition(host.ID, "Customs", 1.5, 2.5, 3.5, 90); err != nil {
		t.Fatalf("SendTestPosition failed: %v", err)
	}

	pos, mapName, _ := host.GetPosition()
	if pos == nil || pos.X != 1.5 || mapName != "Customs" {
		t.Errorf("position not updated: pos=%+v map=%q", pos, mapName)
	}

	if err := h.SendTestPosition("ghost-0000", "Customs", 0, 0, 0, 0); err == nil {
		t.Error("expected error for unknown client")
	}
}

func TestInviteFlowAccept(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	guest := mustCreateTestClient(t, h, "Guest", "guest-0001")

	// Inviter must be in a party
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	h.inviteMu.RLock()
	pending := len(h.invites[guest.ID])
	h.inviteMu.RUnlock()
	if pending != 0 {
		t.Fatalf("invite should not be created when inviter has no party, got %d", pending)
	}

	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Unknown target
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: "ghost-0000"})
	h.inviteMu.RLock()
	pending = len(h.invites["ghost-0000"])
	h.inviteMu.RUnlock()
	if pending != 0 {
		t.Error("invite should not be created for an offline/unknown target")
	}

	// Valid invite -> accept joins the party and consumes the invite
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	h.handleAcceptInvite(guest, &models.AcceptInviteMessage{PartyCode: code})

	if guest.GetParty() != code {
		t.Errorf("guest should be in party %s, got %q", code, guest.GetParty())
	}
	h.inviteMu.RLock()
	remaining := len(h.invites[guest.ID])
	h.inviteMu.RUnlock()
	if remaining != 0 {
		t.Errorf("invite should be consumed, %d remaining", remaining)
	}
}

func TestInviteDeclineAndCancel(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	guest := mustCreateTestClient(t, h, "Guest", "guest-0001")
	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Decline consumes the invite without joining
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	h.handleDeclineInvite(guest, &models.DeclineInviteMessage{PartyCode: code})
	if guest.GetParty() != "" {
		t.Error("guest should not join after declining")
	}
	h.inviteMu.RLock()
	remaining := len(h.invites[guest.ID])
	h.inviteMu.RUnlock()
	if remaining != 0 {
		t.Errorf("declined invite should be consumed, %d remaining", remaining)
	}

	// Cancel: inviter withdraws a pending invite
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	h.handleCancelInvite(host, &models.CancelInviteMessage{TargetClientID: guest.ID})
	h.inviteMu.RLock()
	remaining = len(h.invites[guest.ID])
	h.inviteMu.RUnlock()
	if remaining != 0 {
		t.Errorf("cancelled invite should be removed, %d remaining", remaining)
	}

	// Accepting a nonexistent invite leaves the guest party-less
	h.handleAcceptInvite(guest, &models.AcceptInviteMessage{PartyCode: code})
	if guest.GetParty() != "" {
		t.Error("guest should not join via a cancelled invite")
	}
}

func TestAcceptInviteExpired(t *testing.T) {
	h := newTestHub(t)

	host := mustCreateTestClient(t, h, "Host", "host-00001")
	guest := mustCreateTestClient(t, h, "Guest", "guest-0001")
	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Inject an already-expired invite directly
	h.inviteMu.Lock()
	h.invites[guest.ID] = []*models.PartyInvite{{
		FromClientID: host.ID,
		FromName:     host.DisplayName,
		ToClientID:   guest.ID,
		PartyCode:    code,
		CreatedAt:    time.Now().Add(-10 * time.Minute),
		ExpiresAt:    time.Now().Add(-5 * time.Minute),
	}}
	h.inviteMu.Unlock()

	h.handleAcceptInvite(guest, &models.AcceptInviteMessage{PartyCode: code})
	if guest.GetParty() != "" {
		t.Error("guest should not join via an expired invite")
	}
	h.inviteMu.RLock()
	remaining := len(h.invites[guest.ID])
	h.inviteMu.RUnlock()
	if remaining != 0 {
		t.Errorf("expired invite should be pruned, %d remaining", remaining)
	}
}

func TestFriendRequestForClient(t *testing.T) {
	h := newTestHub(t)

	a := mustCreateTestClient(t, h, "Alice", "client-0001")
	mustCreateTestClient(t, h, "Bob", "client-0002")

	// Unknown clients
	if err := h.SendFriendRequestForClient("ghost-0000", "client-0002"); err == nil {
		t.Error("expected error for unknown sender")
	}
	if err := h.SendFriendRequestForClient("client-0001", "ghost-0000"); err == nil {
		t.Error("expected error for unknown target")
	}

	// Non-test senders are refused
	h.mu.Lock()
	h.clients["real-client"] = &models.Client{ID: "real-client", DisplayName: "Real", ConnectedAt: time.Now()}
	h.mu.Unlock()
	if err := h.SendFriendRequestForClient("real-client", "client-0002"); err == nil {
		t.Error("expected refusal for non-test sender")
	}

	// Happy path
	if err := h.SendFriendRequestForClient(a.ID, "client-0002"); err != nil {
		t.Fatalf("SendFriendRequestForClient failed: %v", err)
	}
	exists, err := h.storage.GetFriendRequest("client-0001", "client-0002")
	if err != nil || !exists {
		t.Fatalf("friend request not stored: exists=%v err=%v", exists, err)
	}

	// Duplicate is rejected
	if err := h.SendFriendRequestForClient(a.ID, "client-0002"); err == nil {
		t.Error("expected duplicate request to be rejected")
	}

	// Reverse request auto-accepts into a friendship
	if err := h.SendFriendRequestForClient("client-0002", a.ID); err != nil {
		t.Fatalf("reverse SendFriendRequestForClient failed: %v", err)
	}
	friends, err := h.storage.AreFriends("client-0001", "client-0002")
	if err != nil || !friends {
		t.Errorf("expected mutual request to auto-accept: friends=%v err=%v", friends, err)
	}

	// Already-friends is rejected
	if err := h.SendFriendRequestForClient(a.ID, "client-0002"); err == nil {
		t.Error("expected already-friends to be rejected")
	}
}

func TestAcceptAndDeclineFriendRequestForClient(t *testing.T) {
	h := newTestHub(t)

	mustCreateTestClient(t, h, "Alice", "client-0001")
	mustCreateTestClient(t, h, "Bob", "client-0002")

	// No request pending
	if err := h.AcceptFriendRequestForClient("client-0002", "client-0001"); err == nil {
		t.Error("expected error when no request exists")
	}

	// Create request A -> B, B accepts
	if err := h.SendFriendRequestForClient("client-0001", "client-0002"); err != nil {
		t.Fatalf("SendFriendRequestForClient failed: %v", err)
	}
	if err := h.AcceptFriendRequestForClient("client-0002", "client-0001"); err != nil {
		t.Fatalf("AcceptFriendRequestForClient failed: %v", err)
	}
	friends, _ := h.storage.AreFriends("client-0001", "client-0002")
	if !friends {
		t.Error("expected friendship after accept")
	}

	// Decline path with a fresh pair
	mustCreateTestClient(t, h, "Carol", "client-0003")
	if err := h.SendFriendRequestForClient("client-0001", "client-0003"); err != nil {
		t.Fatalf("SendFriendRequestForClient failed: %v", err)
	}
	if err := h.DeclineFriendRequestForClient("client-0003", "client-0001"); err != nil {
		t.Fatalf("DeclineFriendRequestForClient failed: %v", err)
	}
	exists, _ := h.storage.GetFriendRequest("client-0001", "client-0003")
	if exists {
		t.Error("declined request should be deleted")
	}
	friends, _ = h.storage.AreFriends("client-0001", "client-0003")
	if friends {
		t.Error("declined pair should not be friends")
	}
}

func TestGetFriendsForClient(t *testing.T) {
	h := newTestHub(t)

	a := mustCreateTestClient(t, h, "Alice", "client-0001")
	b := mustCreateTestClient(t, h, "Bob", "client-0002")

	if err := h.CreateTestFriendship(a.ID, b.ID); err != nil {
		t.Fatalf("CreateTestFriendship failed: %v", err)
	}

	// Put Bob in a party so his status shows InParty
	if _, err := h.CreateTestParty(b.ID, nil); err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	friends, err := h.GetFriendsForClient(a.ID)
	if err != nil || len(friends) != 1 {
		t.Fatalf("GetFriendsForClient failed: %v len=%d", err, len(friends))
	}
	f := friends[0]
	if f.ClientID != b.ID || !f.Online || !f.InParty || f.PartyCode == "" {
		t.Errorf("unexpected friend status: %+v", f)
	}
	if f.DisplayName != "Bob" {
		t.Errorf("expected live display name Bob, got %s", f.DisplayName)
	}

	// Unknown client -> empty list, no error
	friends, err = h.GetFriendsForClient("ghost-0000")
	if err != nil || len(friends) != 0 {
		t.Errorf("expected empty list for unknown client: %v %v", friends, err)
	}

	// Remove friendship
	if err := h.RemoveTestFriendship(a.ID, b.ID); err != nil {
		t.Fatalf("RemoveTestFriendship failed: %v", err)
	}
	friends, _ = h.GetFriendsForClient(a.ID)
	if len(friends) != 0 {
		t.Errorf("expected no friends after removal, got %d", len(friends))
	}
}

func TestGetAllPendingFriendRequests(t *testing.T) {
	h := newTestHub(t)

	mustCreateTestClient(t, h, "Alice", "client-0001")
	mustCreateTestClient(t, h, "Bob", "client-0002")

	if got := h.GetAllPendingFriendRequests(); len(got) != 0 {
		t.Fatalf("expected no pending requests, got %d", len(got))
	}

	if err := h.SendFriendRequestForClient("client-0001", "client-0002"); err != nil {
		t.Fatalf("SendFriendRequestForClient failed: %v", err)
	}

	pending := h.GetAllPendingFriendRequests()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}
	r := pending[0]
	if r.FromClientID != "client-0001" || r.ToClientID != "client-0002" || !r.FromIsTest || !r.ToIsTest {
		t.Errorf("unexpected pending request: %+v", r)
	}
}
