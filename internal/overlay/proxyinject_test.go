package overlay

import (
	"strings"
	"testing"
)

// Offline contract for the inject-script source. The livenet test covers the
// same forbidden list against a live proxy response; these run in CI.

func TestInjectScriptFillsMapAfterConsent(t *testing.T) {
	required := []string{
		"function cookieBannerGone",
		"function mapLooksShort",
		"function fillMapAfterConsent",
		"function watchCookieBanner",
		"fillMapAfterConsent();",
		"watchCookieBanner();",
		"dispatchEvent(new Event('resize'))",
		"--display-height",
		"invalidateSize",
		"ticks >= 10",
		"fetch('/nexus/accept-cookies'",
	}
	for _, needle := range required {
		if !strings.Contains(proxyInjectScript, needle) {
			t.Errorf("inject script missing %q — the cookie-strip happy path is not wired", needle)
		}
	}

	for _, f := range injectScriptForbidden {
		if strings.Contains(proxyInjectScript, f.needle) {
			t.Errorf("inject script contains %q, which %s", f.needle, f.why)
		}
	}
}
