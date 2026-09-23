//go:build windows

package mapwindow

import (
	"path/filepath"
	"testing"
)

func TestResizeCircle_ChangesOnlyOwnedFixture(t *testing.T) {
	// Given a hidden window owned by this test, never a user's map window.
	class := "CircleResizeTest-" + filepath.Base(filepath.Dir(t.TempDir()))
	hwnd := createHiddenWindow(t, class)
	areas, primary, err := Monitors()
	if err != nil {
		t.Fatal(err)
	}
	area := areas[primary]
	dpi, err := WindowDPI(hwnd)
	if err != nil {
		t.Fatal(err)
	}
	// DefWindowProc caps oversized fixtures at the display's tracking limits.
	minimum := (uint64(dpi)*200 + 95) / 96
	requiredShort := int((minimum*2+2)/3+3) / 4 * 4
	short := min(max(240, requiredShort), area.W/3, area.H*2/3)
	short -= short % 4
	if short < requiredShort {
		t.Fatalf("work area %+v at DPI %d cannot fit the proportional resize fixture", area, dpi)
	}
	start := Rect{X: area.X + (area.W-2*short)/2, Y: area.Y + (area.H-short)/2, W: 2 * short, H: short}
	want := Rect{X: start.X - short/2, Y: start.Y - short/4, W: 3 * short, H: 3 * short / 2}
	t.Logf("primary work area=%+v, DPI=%d, resize target=%+v", area, dpi, want)
	if err := SetWindowRect(hwnd, start); err != nil {
		t.Fatal(err)
	}
	other := createHiddenWindow(t, class+"-Other")
	untouched, err := WindowRect(other)
	if err != nil {
		t.Fatal(err)
	}

	// When the visible circle grows by half from its original bounds.
	if err := ResizeCircle(hwnd, start, 1.5); err != nil {
		t.Fatal(err)
	}

	// Then native placement matches the centered proportional rectangle.
	got, err := WindowRect(hwnd)
	if err != nil || got != want {
		t.Fatalf("native resized rectangle = (%+v, %v), want %+v", got, err, want)
	}
	if got, err := WindowRect(other); err != nil || got != untouched {
		t.Fatalf("other window rectangle = (%+v, %v), want unchanged %+v", got, err, untouched)
	}
}

func TestWindowDPI_RejectsMissingWindow(t *testing.T) {
	// Given no native window, as when startup cannot find the owned HWND.
	// When reading the DPI needed to enforce the logical minimum size.
	dpi, err := WindowDPI(0)
	// Then an unavailable native window cannot silently use a guessed DPI.
	if dpi != 0 || err == nil {
		t.Fatalf("WindowDPI(0) = (%d, %v), want error", dpi, err)
	}
}
