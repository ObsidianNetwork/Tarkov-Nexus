//go:build !cgo

// This file is the CGO_ENABLED=0 mirror of the behavioral suite in
// hub_test.go. The "Run party-server tests" steps in build-test.yml gate PRs
// without a C toolchain: go-sqlite3 then compiles its no-CGO stub, and the
// cgo-tagged suites (hub_test.go, storage_test.go) are excluded there. To
// still execute hub behavior in that gate, these tests build the Hub
// directly — no *storage.Storage, because every storage method dereferences
// the SQLite handle — and exercise only the party-coordination paths that
// never touch storage. Paths that need the database (client registration,
// friendships, invite-accept joins) stay in hub_test.go and run in the
// CGO-enabled test job of party-server.yml.

package hub

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/party-server/internal/models"
)

// partyCodePattern is the NATO phonetic WORD-NNNN shape produced by
// generatePartyCode.
var partyCodePattern = regexp.MustCompile(`^[A-Z]+-\d{4}$`)

// newDBFreeHub returns a Hub with empty maps and nil storage. Tests in this
// file must stay on paths that never dereference h.storage.
func newDBFreeHub() *Hub {
	return &Hub{
		clients: make(map[string]*models.Client),
		parties: make(map[string]*models.Party),
		invites: make(map[string][]*models.PartyInvite),
	}
}

// addClient injects a connection-less client directly, the storage-free
// equivalent of CreateTestClient. Nil connections are expected: sendMessage
// and sendError skip them.
func addClient(h *Hub, id, name string) *models.Client {
	c := &models.Client{ID: id, DisplayName: name, ConnectedAt: time.Now()}
	h.mu.Lock()
	h.clients[id] = c
	h.mu.Unlock()
	return c
}

// pendingInvites reports how many unexpired invites a client currently holds.
func pendingInvites(h *Hub, clientID string) int {
	h.inviteMu.RLock()
	defer h.inviteMu.RUnlock()
	return len(h.invites[clientID])
}

// TestDBFreePartyLifecycle pins the create-party path that the CGO_ENABLED=0
// gate executes: unknown hosts are rejected, listed members (and only they)
// join the created party, the code follows the NATO WORD-NNNN shape, and a
// host cannot create a second party while in one.
func TestDBFreePartyLifecycle(t *testing.T) {
	h := newDBFreeHub()

	if _, err := h.CreateTestParty("ghost-0000", nil); err == nil {
		t.Error("expected error for unknown host")
	}

	host := addClient(h, "host-00001", "Host")
	member := addClient(h, "member-001", "Member")
	pending := addClient(h, "member-002", "Pending")

	code, err := h.CreateTestParty(host.ID, []string{member.ID, "ghost-0000"})
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}
	if !partyCodePattern.MatchString(code) {
		t.Errorf("party code %q does not match NATO WORD-NNNN", code)
	}
	if host.GetParty() != code || member.GetParty() != code {
		t.Errorf("host and listed member not attached to party %s", code)
	}
	if pending.GetParty() != "" {
		t.Error("client not listed as a member must stay party-less")
	}

	if _, err := h.CreateTestParty(host.ID, nil); err == nil {
		t.Error("expected error when host is already in a party")
	}

	parties := h.GetAllParties()
	if len(parties) != 1 {
		t.Fatalf("expected 1 party, got %d", len(parties))
	}
	if parties[0].Code != code || parties[0].HostID != host.ID || parties[0].MemberCount != 2 {
		t.Errorf("unexpected party listing: %+v", parties[0])
	}
	if got := h.GetStats()["parties"].(int); got != 1 {
		t.Errorf("expected 1 party in stats, got %d", got)
	}
}

// TestDBFreeJoinLeaveDeleteParty pins join/leave/delete coordination without
// storage: validation errors, position broadcast storing on the sender,
// leave keeping a party alive while members remain, empty-party cleanup, and
// DeleteParty clearing every member's party code.
func TestDBFreeJoinLeaveDeleteParty(t *testing.T) {
	h := newDBFreeHub()

	host := addClient(h, "host-00001", "Host")
	joiner := addClient(h, "member-001", "Joiner")
	late := addClient(h, "member-002", "Late")
	loner := addClient(h, "loner-0001", "Loner")

	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	if err := h.JoinTestParty("ghost-0000", code); err == nil {
		t.Error("expected error for unknown client")
	}
	if err := h.JoinTestParty(late.ID, "NOSUCH-000"); err == nil {
		t.Error("expected error for unknown party")
	}
	if err := h.JoinTestParty(host.ID, code); err == nil {
		t.Error("expected error for client already in a party")
	}

	if err := h.JoinTestParty(joiner.ID, code); err != nil {
		t.Fatalf("JoinTestParty failed: %v", err)
	}
	if joiner.GetParty() != code {
		t.Errorf("joiner not in party %s, got %q", code, joiner.GetParty())
	}

	// Broadcast stores the position on the sender; nil peer connections are
	// skipped silently.
	if err := h.SendTestPosition(joiner.ID, "Customs", 1.5, 2.5, 3.5, 90); err != nil {
		t.Fatalf("SendTestPosition failed: %v", err)
	}
	pos, mapName, _ := joiner.GetPosition()
	if pos == nil || pos.X != 1.5 || mapName != "Customs" {
		t.Errorf("position not stored on sender: pos=%+v map=%q", pos, mapName)
	}
	if err := h.SendTestPosition(loner.ID, "Customs", 0, 0, 0, 0); err == nil {
		t.Error("expected error when sender is not in a party")
	}
	if err := h.SendTestPosition("ghost-0000", "Customs", 0, 0, 0, 0); err == nil {
		t.Error("expected error for unknown sender")
	}

	// Leaving keeps the party alive while the host remains.
	if err := h.LeaveTestParty("ghost-0000"); err == nil {
		t.Error("expected error for unknown client")
	}
	if err := h.LeaveTestParty(loner.ID); err == nil {
		t.Error("expected error for client not in a party")
	}
	if err := h.LeaveTestParty(joiner.ID); err != nil {
		t.Fatalf("LeaveTestParty failed: %v", err)
	}
	if joiner.GetParty() != "" {
		t.Error("joiner should have no party after leaving")
	}
	if got := h.GetStats()["parties"].(int); got != 1 {
		t.Errorf("party should survive while the host remains, got %d parties", got)
	}

	// The last member out deletes the empty party.
	if err := h.LeaveTestParty(host.ID); err != nil {
		t.Fatalf("LeaveTestParty(host) failed: %v", err)
	}
	if got := h.GetStats()["parties"].(int); got != 0 {
		t.Errorf("empty party should be deleted, got %d parties", got)
	}

	// DeleteParty clears every member and forgets the code.
	code2, err := h.CreateTestParty(host.ID, []string{joiner.ID})
	if err != nil {
		t.Fatalf("CreateTestParty(rebuild) failed: %v", err)
	}
	if err := h.DeleteParty(code2); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}
	if host.GetParty() != "" || joiner.GetParty() != "" {
		t.Error("member party codes should be cleared after DeleteParty")
	}
	if err := h.DeleteParty("NOSUCH-000"); err == nil {
		t.Error("expected error for unknown party")
	}
}

// TestDBFreePartyCapacity pins the five-member cap on the join path the gate
// executes: the party fills to five and rejects the sixth joiner.
func TestDBFreePartyCapacity(t *testing.T) {
	h := newDBFreeHub()

	host := addClient(h, "host-00001", "Host")
	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("member-%02d", i)
		addClient(h, id, fmt.Sprintf("Member-%02d", i))
		if err := h.JoinTestParty(id, code); err != nil {
			t.Fatalf("JoinTestParty(%s) should fill the party: %v", id, err)
		}
	}

	sixth := addClient(h, "member-09", "Sixth")
	if err := h.JoinTestParty(sixth.ID, code); err == nil {
		t.Error("expected party-full error for the sixth member")
	}
	if got := h.GetStats()["clients"].(int); got != 6 {
		t.Errorf("expected 6 registered clients, got %d", got)
	}
}

// TestDBFreeInviteDeclineAndCancel pins the invite paths that never touch
// storage: an inviter without a party cannot invite, unknown targets are
// refused, declining or cancelling consumes the pending invite, and accepting
// without a pending invite does not join. (The storage-backed accept-join
// path is covered by hub_test.go under the cgo tag.)
func TestDBFreeInviteDeclineAndCancel(t *testing.T) {
	h := newDBFreeHub()

	host := addClient(h, "host-00001", "Host")
	guest := addClient(h, "guest-0001", "Guest")
	loner := addClient(h, "loner-0001", "Loner")

	// Inviter must be in a party.
	h.handleInviteFriend(loner, &models.InviteFriendMessage{TargetClientID: guest.ID})
	if got := pendingInvites(h, guest.ID); got != 0 {
		t.Fatalf("invite must not be created when the inviter has no party, got %d", got)
	}

	code, err := h.CreateTestParty(host.ID, nil)
	if err != nil {
		t.Fatalf("CreateTestParty failed: %v", err)
	}

	// Unknown target is refused.
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: "ghost-0000"})
	if got := pendingInvites(h, "ghost-0000"); got != 0 {
		t.Errorf("invite must not be created for an unknown target, got %d", got)
	}

	// Valid invite is recorded for the guest.
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	if got := pendingInvites(h, guest.ID); got != 1 {
		t.Fatalf("expected 1 pending invite, got %d", got)
	}

	// Declining consumes the invite without joining.
	h.handleDeclineInvite(guest, &models.DeclineInviteMessage{PartyCode: code})
	if guest.GetParty() != "" {
		t.Error("guest should not join after declining")
	}
	if got := pendingInvites(h, guest.ID); got != 0 {
		t.Errorf("declined invite should be consumed, got %d", got)
	}

	// Re-invite, then the inviter cancels it.
	h.handleInviteFriend(host, &models.InviteFriendMessage{TargetClientID: guest.ID})
	if got := pendingInvites(h, guest.ID); got != 1 {
		t.Fatalf("expected 1 pending invite after re-invite, got %d", got)
	}
	h.handleCancelInvite(host, &models.CancelInviteMessage{TargetClientID: guest.ID})
	if got := pendingInvites(h, guest.ID); got != 0 {
		t.Errorf("cancelled invite should be removed, got %d", got)
	}

	// Accepting without a pending invite must not join the party.
	h.handleAcceptInvite(guest, &models.AcceptInviteMessage{PartyCode: code})
	if guest.GetParty() != "" {
		t.Error("guest should not join without a pending invite")
	}
}

// TestDBFreeClientLookupAndStats pins the read-side surfaces the gate
// executes: zeroed stats on an empty hub, sorted client listings, the
// test-client count, and case-insensitive ID-prefix lookup.
func TestDBFreeClientLookupAndStats(t *testing.T) {
	h := newDBFreeHub()

	stats := h.GetStats()
	if stats["clients"].(int) != 0 || stats["parties"].(int) != 0 || stats["testClients"].(int) != 0 {
		t.Errorf("expected zeroed stats on an empty hub, got %+v", stats)
	}

	addClient(h, "client-0003", "charlie")
	addClient(h, "client-0001", "Alice")
	addClient(h, "client-0002", "bob")

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

	// Only flagged clients count as test clients.
	addClient(h, "bot-00001", "Bot").IsTest = true

	stats = h.GetStats()
	if stats["clients"].(int) != 4 {
		t.Errorf("expected 4 clients, got %v", stats["clients"])
	}
	if stats["testClients"].(int) != 1 {
		t.Errorf("expected 1 test client, got %v", stats["testClients"])
	}

	if got := h.FindClientByPrefix("CLIENT-00"); got != "client-0001" && got != "client-0002" && got != "client-0003" {
		t.Errorf("expected a case-insensitive prefix match, got %q", got)
	}
	if got := h.FindClientByPrefix("zzzz"); got != "" {
		t.Errorf("expected no match, got %q", got)
	}
}
