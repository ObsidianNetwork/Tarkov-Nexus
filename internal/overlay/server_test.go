package overlay

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type nopLogger struct{}

func (nopLogger) Debug(string)   {}
func (nopLogger) Info(string)    {}
func (nopLogger) Warning(string) {}
func (nopLogger) Error(string)   {}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func serverReady(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func startOverlay(t *testing.T) (*Server, int) {
	t.Helper()
	log := nopLogger{}
	for i := 0; i < 8; i++ {
		port := freePort(t)
		s := NewServer(port, log)
		if err := s.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		if serverReady(addr, 400*time.Millisecond) {
			t.Cleanup(func() { _ = s.Stop() })
			return s, port
		}
		_ = s.Stop()
	}
	t.Fatal("could not start overlay server")
	return nil, 0
}

func dialOverlay(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial overlay: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func drain(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// TestConcurrentBroadcastDoesNotPanic is the party-map crash: two members
// send position updates (or a map change overlaps a position broadcast)
// while the overlay viewer is connected. gorilla/websocket panics on
// concurrent WriteMessage; an unrecovered panic in a broadcast goroutine
// takes down the whole desktop app.
func TestConcurrentBroadcastDoesNotPanic(t *testing.T) {
	s, port := startOverlay(t)
	conn := dialOverlay(t, port)
	go drain(conn)

	// Wait until the server has registered the client so broadcasts go somewhere.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.clientsMu.RLock()
		n := len(s.clients)
		s.clientsMu.RUnlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines + 2)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			s.Broadcast(map[string]interface{}{
				"type": "partyPosition",
				"data": map[string]interface{}{
					"remoteId":    fmt.Sprintf("p%d", i),
					"displayName": "Player",
					"position":    map[string]interface{}{"x": float64(i), "y": 0.0, "z": 1.0},
					"rotation":    0.0,
					"map":         "customs",
				},
			})
			s.SetCurrentMap("customs")
		}(i)
	}
	// Overlap a second viewer connect (sendFullState) and /nexus/state with broadcasts.
	go func() {
		defer wg.Done()
		url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			return
		}
		_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, _, _ = c.ReadMessage()
		_ = c.Close()
	}()
	go func() {
		defer wg.Done()
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/nexus/state", port))
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}()
	wg.Wait()
}

func TestBroadcastUpdatesPartyState(t *testing.T) {
	s, _ := startOverlay(t)

	s.Broadcast(map[string]interface{}{
		"type": "partyPosition",
		"data": map[string]interface{}{
			"remoteId":    "abc",
			"displayName": "Alice",
			"position":    map[string]interface{}{"x": 1.0, "y": 2.0, "z": 3.0},
			"rotation":    45.0,
			"map":         "customs",
		},
	})

	s.stateMu.RLock()
	pp := s.partyPositions["abc"]
	s.stateMu.RUnlock()
	if pp == nil {
		t.Fatal("expected party position for abc to be stored")
	}
	if pp.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice", pp.DisplayName)
	}

	s.Broadcast(map[string]interface{}{
		"type": "partyMemberLeft",
		"data": map[string]interface{}{"remoteId": "abc"},
	})

	s.stateMu.RLock()
	_, exists := s.partyPositions["abc"]
	s.stateMu.RUnlock()
	if exists {
		t.Error("party position for abc should be removed after partyMemberLeft")
	}
}

// TestAcceptsGzip covers the cases a substring check gets wrong. "gzip;q=0"
// names gzip in order to refuse it, and "*" accepts it without naming it.
// Getting this wrong means forwarding compressed bytes to a caller that said it
// could not decode them.
func TestAcceptsGzip(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"GZIP", true},
		{" gzip ; q=0.8 ", true},
		{"gzip;q=0", false},   // explicit refusal
		{"gzip;q=0.0", false}, // same, spelled out
		{"deflate, gzip;q=0", false},
		{"identity", false},
		{"deflate, br", false},
		{"*", true}, // accepted without being named
		{"*;q=0", false},
		{"gzip;q=0, *", false}, // explicit gzip entry wins over the wildcard
		{"*, gzip", true},
	} {
		if got := acceptsGzip(tc.header); got != tc.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}
