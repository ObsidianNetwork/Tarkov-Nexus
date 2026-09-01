package integration

import (
	"strings"
	"sync"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/internal/config"
	"tarkov-screenshot-analyzer/internal/position"
)

// capturingLogger is an in-memory logger.Logger that retains every entry the
// Integration emits, so a failing scenario leaves readable, attributable
// evidence in `go test -v` output (see dump). It keeps these tests hermetic:
// entries stay in memory — no console output, no files, no network — until
// the test itself chooses to print them. This is the headless diagnostic
// route for the screenshot chain: the same entries the in-app Logs viewer
// would show, captured without launching the GUI.
type capturingLogger struct {
	mu      sync.Mutex
	entries []capturedEntry
}

type capturedEntry struct {
	level   string
	message string
}

func (l *capturingLogger) log(level, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, capturedEntry{level: level, message: message})
}

func (l *capturingLogger) Debug(message string)   { l.log("DEBUG", message) }
func (l *capturingLogger) Info(message string)    { l.log("INFO", message) }
func (l *capturingLogger) Warning(message string) { l.log("WARNING", message) }
func (l *capturingLogger) Error(message string)   { l.log("ERROR", message) }

// has reports whether any retained entry at the given level contains message.
func (l *capturingLogger) has(level, message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.level == level && strings.Contains(e.message, message) {
			return true
		}
	}
	return false
}

// dump prints every retained entry as test output, attributed by level.
func (l *capturingLogger) dump(t *testing.T) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		t.Log("(no log entries captured)")
		return
	}
	for _, e := range l.entries {
		t.Logf("[%s] %s", e.level, e.message)
	}
}

// newTestIntegration builds an Integration without calling Initialize(), so no
// WebSocket connection, screenshot watcher, or game-log watcher is started.
// wsClient and the other components stay nil, which also exercises the
// nil-guards that the public methods must apply before Initialize() runs.
// The returned capturingLogger retains everything the Integration logs.
func newTestIntegration() (*Integration, *capturingLogger) {
	logs := &capturingLogger{}
	return New(&config.Config{}, logs), logs
}

// TestSetManualMapOverrideRejectsInvalidMap pins the validation guard in
// SetManualMapOverride: a map name the resolver does not recognize must be
// rejected and must not become the stored override. This is the check that stops
// an invalid map name from reaching the tarkov.dev navigation call.
//
// MapResolver.FindByNameID matches nameIDs and normalized names
// case-insensitively, but does not trim or accept display names — so a real map
// referred to by its human label or with stray whitespace is still rejected.
//
// "streets-of-tarkov" used to be listed here as invalid. That was wrong: it is
// the value tarkov.dev itself uses, it is what the map selector supplies, and
// refusing it is what made the manual override unusable for that map. It is now
// covered as a valid input by TestSetManualMapOverrideNormalizesValidNameID.
func TestSetManualMapOverrideRejectsInvalidMap(t *testing.T) {
	testCases := []struct {
		name    string
		mapName string
	}{
		{name: "Unknown map name", mapName: "atlantis"},
		{name: "Display name is not a valid nameID", mapName: "Streets of Tarkov"},
		{name: "Spaced variant of a nameID", mapName: "ground zero"},
		{name: "Whitespace only", mapName: "   "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			i, _ := newTestIntegration()

			i.SetManualMapOverride(tc.mapName)

			if got := i.GetManualMapOverride(); got != "" {
				t.Errorf("SetManualMapOverride(%q) stored an invalid override: got %q, want %q",
					tc.mapName, got, "")
			}
		})
	}
}

// TestSetManualMapOverrideNormalizesValidNameID is the other half of the guard:
// a recognized name is accepted in any casing and stored in tarkov.dev's
// normalized form. It also pins the nil-client guard on the accept path:
// without Initialize(), i.wsClient is nil and must not be dereferenced when a
// valid map is set.
//
// This assertion used to require the input be stored verbatim. That was the
// defect: the stored value is forwarded to SendMapChange and used as the map on
// position updates, and tarkov.dev accepts only normalized names — so an
// override of "tarkovstreets" or "sandbox" was refused on the wire and the
// feature silently did nothing.
func TestSetManualMapOverrideNormalizesValidNameID(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"customs", "customs"},
		{"Customs", "customs"},
		{"tarkovstreets", "streets-of-tarkov"},
		{"streets", "streets-of-tarkov"},
		{"sandbox", "ground-zero"},
		{"ground-zero", "ground-zero"},
		{"streets-of-tarkov", "streets-of-tarkov"}, // already normalized
	} {
		t.Run(tc.in, func(t *testing.T) {
			i, _ := newTestIntegration()

			i.SetManualMapOverride(tc.in)

			if got := i.GetManualMapOverride(); got != tc.want {
				t.Errorf("SetManualMapOverride(%q) stored %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSetManualMapOverrideAcceptsEmptyStringAsClear confirms the empty string is
// treated as "no override" rather than rejected as an invalid map.
func TestSetManualMapOverrideAcceptsEmptyStringAsClear(t *testing.T) {
	i, _ := newTestIntegration()

	i.SetManualMapOverride("")

	if got := i.GetManualMapOverride(); got != "" {
		t.Errorf("SetManualMapOverride(\"\") should leave the override empty, got %q", got)
	}
}

// TestClearEventHandlersStopsDelivery pins the leak guard used on restart:
// after ClearEventHandlers, a previously registered handler must no longer
// receive emitted events. Without this, restarting the integration would deliver
// each event to every handler registered by an earlier run.
func TestClearEventHandlersStopsDelivery(t *testing.T) {
	i, _ := newTestIntegration()

	// Buffered so a stray delivery cannot block emit's goroutine.
	received := make(chan interface{}, 4)
	i.OnEvent("connected", func(data interface{}) {
		received <- data
	})

	i.emit("connected", "first")

	select {
	case got := <-received:
		if got != "first" {
			t.Fatalf("handler received %v, want %q", got, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registered handler did not receive the emitted event")
	}

	i.ClearEventHandlers()
	i.emit("connected", "second")

	select {
	case got := <-received:
		t.Errorf("handler still received %v after ClearEventHandlers", got)
	case <-time.After(200 * time.Millisecond):
		// Expected: no delivery after handlers are cleared.
	}
}

// TestPublicMethodsBeforeInitialize pins the nil-guards on the Integration
// methods that consult components created by Initialize(): none of them may
// panic when called on a freshly constructed Integration.
func TestPublicMethodsBeforeInitialize(t *testing.T) {
	i, _ := newTestIntegration()

	// GetStatus reports idle values instead of dereferencing nil components.
	status := i.GetStatus()
	for _, key := range []string{"initialized", "connected", "monitoring"} {
		if v, ok := status[key].(bool); !ok || v {
			t.Errorf("GetStatus()[%q] = %v, want false", key, status[key])
		}
	}

	// Connect reports an error instead of dereferencing the nil client.
	if err := i.Connect(); err == nil {
		t.Error("Connect() before Initialize() should return an error, got nil")
	}

	// Disconnect is a no-op instead of a nil dereference.
	i.Disconnect()

	// InjectPosition surfaces the not-connected error from updatePosition.
	if err := i.InjectPosition(&position.PositionData{X: 1, Y: 2, Z: 3, Map: "customs"}); err == nil {
		t.Error("InjectPosition() before Initialize() should return an error, got nil")
	}
}

// TestSetManualMapOverrideLeavesLogEvidence pins the headless diagnostic
// route for the map-decision chain: trigger (override request) -> decision
// (name validation) -> failure (invalid map rejected) -> recovery (valid map
// accepted) -> result (override stored, then cleared) must be retained as
// readable, attributable log entries. A regression in this chain can then be
// localized from `go test -v` output alone, without the in-app Logs viewer,
// while the tests stay hermetic (no network, no GUI).
func TestSetManualMapOverrideLeavesLogEvidence(t *testing.T) {
	i, logs := newTestIntegration()

	// Failure: an unrecognized map name is rejected and named in a warning.
	i.SetManualMapOverride("atlantis")
	if !logs.has("WARNING", "Invalid map name for manual override: atlantis") {
		logs.dump(t)
		t.Error("rejecting an invalid map must leave a WARNING entry naming the map")
	}

	// Recovery: a recognized nameID is accepted and named in an info entry.
	i.SetManualMapOverride("customs")
	if !logs.has("INFO", "Manual map override set to: customs") {
		logs.dump(t)
		t.Error("accepting a valid map must leave an INFO entry naming the map")
	}

	// Result: the override is stored, and a later clear is logged too.
	if got := i.GetManualMapOverride(); got != "customs" {
		logs.dump(t)
		t.Errorf("override should be %q after recovery, got %q", "customs", got)
	}
	i.SetManualMapOverride("")
	if !logs.has("DEBUG", "Manual map override cleared") {
		logs.dump(t)
		t.Error("clearing the override must leave a DEBUG entry")
	}

	// Retain the full evidence trail in the test output.
	logs.dump(t)
}
