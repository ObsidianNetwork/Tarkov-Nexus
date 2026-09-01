//go:build livenet

// Live network test for the tarkov.dev reverse proxy.
//
// Excluded from normal runs by the `livenet` build tag because it makes real
// requests to tarkov.dev. Run it deliberately:
//
//	go test -tags livenet -count=1 -v ./internal/overlay/ -run TestLiveProxy
//
// It exists because the proxy's whole job is rewriting someone else's
// responses, and that behaviour cannot be verified against a stub — the
// assertions below are about what tarkov.dev actually sends today.
//
// Background: an earlier version of this proxy stripped tarkov.dev's
// Content-Security-Policy and X-Content-Type-Options, forced a wildcard CORS
// header onto their responses, answered their cookie-consent prompt on the
// user's behalf, hid their branding, unchecked their map filters, and replaced
// window.L.Map wholesale. See docs/tarkov-dev-remediation.md and
// docs/plans/map-window-redesign/02-architecture.md.

package overlay

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	livePort  = 44455 // deliberately not 44444: a running TarkovNexus.exe holds that
	livePath  = "/map/customs"
	browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

type testLogger struct{ t *testing.T }

func (l testLogger) Debug(string)     {}
func (l testLogger) Info(m string)    { l.t.Logf("info: %s", m) }
func (l testLogger) Warning(m string) { l.t.Logf("warn: %s", m) }
func (l testLogger) Error(m string)   { l.t.Errorf("server error: %s", m) }

// upstreamHeader fetches the same path straight from tarkov.dev, so a header
// they set themselves can be told apart from one the proxy invents.
func upstreamHeader(t *testing.T, name string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", "https://tarkov.dev"+livePath, nil)
	req.Header.Set("User-Agent", browserUA)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("upstream fetch failed: %v", err)
	}
	defer resp.Body.Close()
	return resp.Header.Get(name)
}

func TestLiveProxy(t *testing.T) {
	// Fail loudly rather than silently testing somebody else's server: the
	// overlay Server logs bind errors from a goroutine and Start() still
	// returns nil, so a port clash otherwise looks like a passing run against
	// whatever process already owns the port.
	probe, err := (&http.Client{Timeout: 2 * time.Second}).Get(
		fmt.Sprintf("http://127.0.0.1:%d/nexus/state", livePort))
	if err == nil {
		probe.Body.Close()
		t.Fatalf("port %d is already in use — stop that process first, "+
			"otherwise this test would exercise it instead of the code under test", livePort)
	}

	s := NewServer(livePort, testLogger{t})
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()
	time.Sleep(600 * time.Millisecond)

	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d%s", livePort, livePath), nil)
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", fmt.Sprintf("http://localhost:%d%s", livePort, livePath))

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("request through proxy failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	html := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d — Cloudflare may be challenging this client; "+
			"the remaining assertions would be meaningless", resp.StatusCode)
	}
	t.Logf("proxied %s: %d bytes", livePath, len(html))

	// ── Headers: remove exactly one, preserve everything else ──────────────

	if got := resp.Header.Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options = %q, want removed (tarkov.dev sends DENY; "+
			"the iframe cannot load while it is present)", got)
	}
	// Removing X-Frame-Options must be paired with a frame-ancestors policy,
	// otherwise any page the user visits can embed the proxy.
	csps := resp.Header.Values("Content-Security-Policy")
	foundFrameAncestors := false
	for _, v := range csps {
		if strings.Contains(v, "frame-ancestors") {
			foundFrameAncestors = true
			if !strings.Contains(v, "wails.localhost") {
				t.Errorf("frame-ancestors policy %q does not allow the Wails map window origin", v)
			}
		}
	}
	if !foundFrameAncestors {
		t.Error("X-Frame-Options is removed but no frame-ancestors policy was added: " +
			"any site the user visits could frame this proxy")
	}

	// Compare values, not just presence: a proxy that replaced their CSP with a
	// weaker one would satisfy a non-empty check while quietly downgrading the
	// user's protection. These must arrive byte-identical to upstream.
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options"} {
		// Header.Get returns the first value, which is upstream's — the
		// frame-ancestors policy we append is a separate header and is asserted
		// above.
		got, want := resp.Header.Get(h), upstreamHeader(t, h)
		if want == "" {
			t.Errorf("upstream no longer sends %s; this assertion needs revisiting", h)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, upstream sends %q — it must pass through unmodified", h, got, want)
		}
	}
	if got, want := resp.Header.Get("Access-Control-Allow-Origin"), upstreamHeader(t, "Access-Control-Allow-Origin"); got != want {
		t.Errorf("Access-Control-Allow-Origin = %q, upstream sends %q — "+
			"their CORS policy must pass through unmodified", got, want)
	}

	// ── Injected script: present, and no longer invasive ───────────────────

	// Scope these to the script we author. Matching the whole page would search
	// tarkov.dev's own bundles too — "savedMapSettings" is *their* settings key,
	// so if their frontend ships that literal the test would fail while the
	// proxy is behaving perfectly.
	forbidden := []struct{ needle, why string }{
		{"CookieConsent=true", "answers tarkov.dev's cookie consent on the user's behalf"},
		{".CookieConsent { display: none", "suppresses the consent banner so the user cannot answer it"},
		{".id-wrapper { display: none", "hides tarkov.dev's own branding and session widget"},
		{"window.L.Map = function", "replaces Leaflet's Map constructor; use L.Map.addInitHook"},
		{"savedMapSettings", "overrides the user's own map settings on tarkov.dev"},
		{"cb.click()", "programmatically unchecks tarkov.dev's map filters"},
	}
	for _, f := range forbidden {
		if strings.Contains(proxyInjectScript, f.needle) {
			t.Errorf("the injected script contains %q, which %s", f.needle, f.why)
		}
	}

	// The page must actually carry our script, and that script must use the
	// supported hook. The first is about the proxy, the second about the script.
	if !strings.Contains(html, "_nexusPartyInjected") {
		t.Error("proxied page is missing the party-marker script")
	}
	for _, needle := range []string{"addInitHook", "L.Layer.prototype.addTo"} {
		if !strings.Contains(proxyInjectScript, needle) {
			t.Errorf("injected script is missing %q: map capture needs both the init "+
				"hook and the late-capture fallback for maps built before it ran", needle)
		}
	}
}
