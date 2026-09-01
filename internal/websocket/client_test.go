package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"tarkov-screenshot-analyzer/internal/game"

	"github.com/gorilla/websocket"
)

// These tests pin the wire contract against the real tarkov.dev socket server,
// the-hideout/tarkov-socket-server (server.mjs). That server routes on
// message.type and warns on anything it does not recognise:
//
//	if (message.type === 'pong')    { ws.isAlive = true; return; }
//	if (message.type === 'command') { sendMessage(sessionID, 'command', message.data); return; }
//	if (message.type === 'debug')   { sendMessage(sessionID, 'debug',   message.data); return; }
//	console.warn(`Unrecognized message type ...`);
//
// Sending a type outside that set produced the server-side warnings that got
// this application blocked (Linear TAR-2). These tests exist so that cannot
// silently come back.

// serverAccepts mirrors the routing table in server.mjs.
var serverAccepts = map[string]bool{"command": true, "debug": true, "pong": true}

type nopLogger struct{}

func (nopLogger) Debug(string)   {}
func (nopLogger) Info(string)    {}
func (nopLogger) Warning(string) {}
func (nopLogger) Error(string)   {}

// testServer stands in for the tarkov.dev socket server. It records the
// handshake headers and every message the client sends.
type testServer struct {
	srv       *httptest.Server
	url       string
	handshake chan *http.Request
	messages  chan map[string]interface{}
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ts := &testServer{
		handshake: make(chan *http.Request, 1),
		messages:  make(chan map[string]interface{}, 16),
	}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	ts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case ts.handshake <- r.Clone(r.Context()):
		default:
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var m map[string]interface{}
			if err := conn.ReadJSON(&m); err != nil {
				return
			}
			ts.messages <- m
		}
	}))
	ts.url = "ws" + strings.TrimPrefix(ts.srv.URL, "http")
	t.Cleanup(ts.srv.Close)
	return ts
}

// connectTo dials the test server, bypassing the hard-coded tarkov.dev URL in
// Connect() while exercising the same headers Connect() sets.
func connectTo(t *testing.T, ts *testServer, remoteID string) *Client {
	t.Helper()
	c := NewClient(remoteID, time.Second, 1, nopLogger{})

	header := http.Header{}
	header.Set("Origin", clientName)

	conn, _, err := websocket.DefaultDialer.Dial(ts.url, header)
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	c.conn = conn
	t.Cleanup(func() { conn.Close() })
	return c
}

func nextMessage(t *testing.T, ts *testServer) map[string]interface{} {
	t.Helper()
	select {
	case m := <-ts.messages:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
		return nil
	}
}

// TestConnectSendsIdentifyingHeaders covers Razzmatazzz's condition 3: the app
// must identify itself to the socket server. Their server logs
// `Client connected ${sessionID} from ${req.headers.origin}` — without this
// header every connection logged "from undefined", which is precisely why the
// traffic could not be attributed.
//
// This calls the real Connect() rather than dialling itself. An earlier version
// used the test harness's own dial, which set the Origin header in the test and
// then asserted on it — so deleting the header from Connect() would not have
// failed anything, and the guarantee this test exists to protect was unguarded.
func TestConnectSendsIdentifyingHeaders(t *testing.T) {
	ts := newTestServer(t)

	c := NewClient("AB12", time.Second, 1, nopLogger{})
	c.dialURL = ts.url
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(c.Disconnect)

	var req *http.Request
	select {
	case req = <-ts.handshake:
	case <-time.After(2 * time.Second):
		t.Fatal("no handshake observed")
	}

	if got := req.Header.Get("Origin"); got != "TarkovNexus" {
		t.Errorf("Origin header = %q, want %q — tarkov.dev cannot attribute the traffic without it", got, "TarkovNexus")
	}

	ua := req.Header.Get("User-Agent")
	if !strings.HasPrefix(ua, "TarkovNexus/") {
		t.Errorf("User-Agent = %q, want a TarkovNexus/<version> value", ua)
	}
	if ua == "TarkovNexus/" {
		t.Error("User-Agent carries no version")
	}
}

// TestOriginKeepsNativePing guards a subtle coupling in server.mjs:
//
//	if (!req.headers.origin?.startsWith('http')) { ws.nativePing = true; }
//
// "TarkovNexus" does not start with "http", so the server uses protocol-level
// ping frames, which gorilla/websocket answers automatically. Changing the
// Origin to a URL would silently flip the server onto JSON pings.
func TestOriginKeepsNativePing(t *testing.T) {
	if strings.HasPrefix(clientName, "http") {
		t.Fatalf("clientName %q starts with \"http\": this flips tarkov.dev onto JSON pings", clientName)
	}
}

// TestSentMessagesUseOnlyRecognizedTypes is the regression test for the actual
// incident. Every message the client can emit must carry a type the server
// routes; anything else lands in `console.warn("Unrecognized message type")`.
func TestSentMessagesUseOnlyRecognizedTypes(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	if err := c.SendMapChange("customs"); err != nil {
		t.Fatalf("SendMapChange: %v", err)
	}
	if err := c.SendPositionUpdate(1.5, 2.5, 3.5, 180, "customs"); err != nil {
		t.Fatalf("SendPositionUpdate: %v", err)
	}
	if err := c.SendPong(); err != nil {
		t.Fatalf("SendPong: %v", err)
	}

	for i := 0; i < 3; i++ {
		msg := nextMessage(t, ts)
		typ, _ := msg["type"].(string)
		if !serverAccepts[typ] {
			t.Errorf("message %d has type %q, which tarkov.dev's server does not recognise "+
				"(it accepts only command/debug/pong and warns on everything else): %v", i, typ, msg)
		}
	}
}

// TestNoPartyMessageSenders is a source-level guard. The party senders were
// removed because they emitted type:"party". A compile-time reference keeps
// anyone from reintroducing them without this test failing to build.
func TestNoPartyMessageSenders(t *testing.T) {
	var c interface{} = (*Client)(nil)
	if _, bad := c.(interface {
		SendPartyPositionUpdate(string, string, float64, float64, float64, float64, string) error
	}); bad {
		t.Error("SendPartyPositionUpdate is back on *Client; it emits type:\"party\", " +
			"which tarkov.dev's socket server does not recognise (see TAR-2)")
	}
	if _, bad := c.(interface{ SendPartyMemberLeft(string) error }); bad {
		t.Error("SendPartyMemberLeft is back on *Client; it emits type:\"party\" (see TAR-2)")
	}
}

// TestMapChangeWireFormat pins the exact envelope, which must stay
// byte-compatible with TarkovMonitor's — that is what the website consumes.
func TestMapChangeWireFormat(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	if err := c.SendMapChange("streets-of-tarkov"); err != nil {
		t.Fatalf("SendMapChange: %v", err)
	}
	msg := nextMessage(t, ts)

	if msg["sessionID"] != "AB12" {
		t.Errorf("sessionID = %v, want AB12 (the recipient's ID, no -tm suffix)", msg["sessionID"])
	}
	if msg["type"] != "command" {
		t.Errorf("type = %v, want command", msg["type"])
	}
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %v", msg["data"])
	}
	if data["type"] != "map" || data["value"] != "streets-of-tarkov" {
		t.Errorf("data = %v, want {type:map, value:streets-of-tarkov}", data)
	}
}

// TestPositionUpdateWireFormat pins the playerPosition envelope. tarkov.dev
// renders this natively (src/pages/map/index.js) — it is the supported path.
func TestPositionUpdateWireFormat(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	if err := c.SendPositionUpdate(-14.25, 2.5, 118.75, 183.5, "customs"); err != nil {
		t.Fatalf("SendPositionUpdate: %v", err)
	}
	msg := nextMessage(t, ts)

	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %v", msg["data"])
	}
	if data["type"] != "playerPosition" {
		t.Errorf("data.type = %v, want playerPosition", data["type"])
	}
	if data["map"] != "customs" {
		t.Errorf("data.map = %v, want customs", data["map"])
	}
	pos, ok := data["position"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.position is not an object: %v", data["position"])
	}
	for key, want := range map[string]float64{"x": -14.25, "y": 2.5, "z": 118.75} {
		if got, _ := pos[key].(float64); got != want {
			t.Errorf("position.%s = %v, want %v", key, pos[key], want)
		}
	}
	if got, _ := data["rotation"].(float64); got != 183.5 {
		t.Errorf("rotation = %v, want 183.5", data["rotation"])
	}
}

// TestPongIsBare matches the website client, which replies with exactly
// {"type":"pong"} and no session ID.
func TestPongIsBare(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	if err := c.SendPong(); err != nil {
		t.Fatalf("SendPong: %v", err)
	}
	msg := nextMessage(t, ts)

	if msg["type"] != "pong" {
		t.Errorf("type = %v, want pong", msg["type"])
	}
	if _, present := msg["sessionID"]; present {
		t.Errorf("pong carries a sessionID; the website sends a bare pong: %v", msg)
	}
}

// TestEmptyMapIsRefused: a command whose value is blank would navigate a paired
// browser to /map/ and is never useful. Refuse it before it reaches the wire.
func TestEmptyMapIsRefused(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	if err := c.SendMapChange(""); err == nil {
		t.Error("SendMapChange(\"\") should refuse an empty map name")
	}
	if err := c.SendPositionUpdate(1, 2, 3, 0, ""); err == nil {
		t.Error("SendPositionUpdate with an empty map name should be refused")
	}

	select {
	case m := <-ts.messages:
		t.Errorf("a message reached the server despite an empty map name: %v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestNotConnectedIsRefused: every sender must fail closed rather than
// dereference a nil connection.
func TestNotConnectedIsRefused(t *testing.T) {
	c := NewClient("AB12", time.Second, 1, nopLogger{})

	if err := c.SendMapChange("customs"); err == nil {
		t.Error("SendMapChange should fail when not connected")
	}
	if err := c.SendPositionUpdate(1, 2, 3, 0, "customs"); err == nil {
		t.Error("SendPositionUpdate should fail when not connected")
	}
	if err := c.SendPong(); err == nil {
		t.Error("SendPong should fail when not connected")
	}
}

// TestMessageEnvelopeOmitsEmptyData documents why `data` is omitempty: a bare
// pong must not serialise a null data field.
func TestMessageEnvelopeOmitsEmptyData(t *testing.T) {
	b, err := json.Marshal(Message{SessionID: "AB12", Type: "command"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "data") {
		t.Errorf("empty Data was serialised: %s", b)
	}
}

// TestKnownMapsMatchesTarkovDev pins our allowlist to tarkov.dev's own data.
// The list is transcribed from the normalizedName values in
// research/tarkov-dev/src/data/maps.json; if that vendored copy is refreshed
// and the sets drift, this fails rather than letting us send a map they do not
// have (or silently refuse one they do).
func TestKnownMapsMatchesTarkovDev(t *testing.T) {
	const mapsJSON = "../../research/tarkov-dev/src/data/maps.json"

	raw, err := os.ReadFile(mapsJSON)
	if err != nil {
		t.Skipf("vendored tarkov.dev data not present (%v); skipping", err)
	}
	var entries []struct {
		NormalizedName string `json:"normalizedName"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s: %v", mapsJSON, err)
	}
	if len(entries) == 0 {
		t.Fatalf("%s parsed to zero entries", mapsJSON)
	}

	theirs := map[string]bool{}
	for _, e := range entries {
		theirs[e.NormalizedName] = true
	}
	for name := range theirs {
		if !knownMaps[name] {
			t.Errorf("tarkov.dev has map %q but knownMaps does not — we would refuse a valid map", name)
		}
	}
	for name := range knownMaps {
		if !theirs[name] {
			t.Errorf("knownMaps has %q but tarkov.dev does not — sending it would navigate their site to a 404", name)
		}
	}
}

// TestInvalidMapNamesAreRefused covers the two names that actually shipped:
// "labs" from internal/game/map_resolver.go and "night-factory" from
// internal/maps/resolver.go, the latter on every night Factory raid.
func TestInvalidMapNamesAreRefused(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	for _, bad := range []string{"labs", "night-factory", "Customs", "customs ", "the_lab", "/map/customs"} {
		if err := c.SendMapChange(bad); err == nil {
			t.Errorf("SendMapChange(%q) was allowed; tarkov.dev has no such map", bad)
		}
		if err := c.SendPositionUpdate(1, 2, 3, 0, bad); err == nil {
			t.Errorf("SendPositionUpdate with map %q was allowed", bad)
		}
	}

	select {
	case m := <-ts.messages:
		t.Errorf("an invalid map reached the server: %v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestValidMapNamesAreAccepted guards against the opposite failure: a guard so
// strict it blocks real maps.
func TestValidMapNamesAreAccepted(t *testing.T) {
	ts := newTestServer(t)
	c := connectTo(t, ts, "AB12")

	for name := range knownMaps {
		if err := c.SendMapChange(name); err != nil {
			t.Errorf("SendMapChange(%q) refused a valid tarkov.dev map: %v", name, err)
			continue
		}
		msg := nextMessage(t, ts)
		data, _ := msg["data"].(map[string]interface{})
		if data["value"] != name {
			t.Errorf("sent map %q but wire carried %v", name, data["value"])
		}
	}
}

// TestResolverOutputIsAlwaysSendable closes the loop between the map resolvers
// and this package's allowlist. The resolvers decide what map name reaches the
// wire; knownMaps decides what is allowed on it. When those two disagree the
// failure is silent — the send is refused and the feature simply stops working,
// which is exactly what happened when the dashboard selector handed back game
// name IDs ("streets", "sandbox") that checkMapName rejected.
func TestResolverOutputIsAlwaysSendable(t *testing.T) {
	resolver := game.NewMapResolver()

	for _, m := range resolver.GetAllMaps() {
		if m.NormalizedName == "" {
			t.Errorf("resolver entry %q has an empty normalized name", m.NameID)
			continue
		}
		if !knownMaps[m.NormalizedName] {
			t.Errorf("resolver maps %q -> %q, which knownMaps rejects: this map "+
				"silently stops working on tarkov.dev", m.NameID, m.NormalizedName)
		}

		// A normalized name must also survive a second pass through the
		// resolver, since the dashboard selector round-trips through it.
		if got := resolver.GetNormalizedName(m.NormalizedName); got != m.NormalizedName {
			t.Errorf("resolver does not accept its own output %q (got %q); the map "+
				"selector shows a blank entry and the manual override is refused",
				m.NormalizedName, got)
		}
	}

	// The manual selector is the other path onto the wire: whatever it offers
	// the user must be sendable and must render with a label.
	selectable := resolver.GetSelectableMaps()
	if len(selectable) == 0 {
		t.Fatal("GetSelectableMaps returned nothing; the manual map selector would be empty")
	}
	seen := map[string]bool{}
	for _, m := range selectable {
		if !knownMaps[m.NormalizedName] {
			t.Errorf("selector offers %q, which knownMaps rejects: picking it does nothing", m.NormalizedName)
		}
		if m.DisplayName == "" {
			t.Errorf("selector entry %q has no display name; it renders blank", m.NormalizedName)
		}
		if seen[m.NormalizedName] {
			t.Errorf("selector lists %q more than once", m.NormalizedName)
		}
		seen[m.NormalizedName] = true
	}
}
