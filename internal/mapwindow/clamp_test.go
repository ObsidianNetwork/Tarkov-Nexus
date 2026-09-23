package mapwindow

import "testing"

// Work areas copied from the development rig (physical pixels, taskbar
// excluded), as read by a per-monitor-DPI-aware process:
//
//	primary  3440x1392 at (0,0)
//	left     2560x1392 at (-2560,77)   negative X
//	right    2880x1560 at (3440,-95)   negative Y, 125% DPI
var (
	rigPrimary = Rect{X: 0, Y: 0, W: 3440, H: 1392}
	rigLeft    = Rect{X: -2560, Y: 77, W: 2560, H: 1392}
	rigRight   = Rect{X: 3440, Y: -95, W: 2880, H: 1560}
	rig        = []Rect{rigLeft, rigPrimary, rigRight}
	rigPrimIdx = 1
)

func inside(r, area Rect) bool {
	return r.X >= area.X && r.Y >= area.Y && r.Right() <= area.Right() && r.Bottom() <= area.Bottom()
}

func TestClamp_NilSavedCentresDefaultOnPrimary(t *testing.T) {
	got := Clamp(nil, rig, rigPrimIdx)
	want := Rect{X: (3440 - 500) / 2, Y: (1392 - 500) / 2, W: 500, H: 500}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestClamp_FullyInsideMonitorIsUnchanged(t *testing.T) {
	saved := Rect{X: -2100, Y: 350, W: 560, H: 600}
	if got := Clamp(&saved, rig, rigPrimIdx); got != saved {
		t.Fatalf("got %+v, want unchanged %+v", got, saved)
	}
}

func TestClamp_PartiallyOffRightEdgeSlidesLeft(t *testing.T) {
	// 300px hanging off the right of the primary, no monitor there.
	saved := Rect{X: 3440 - 200, Y: 100, W: 500, H: 500}
	mons := []Rect{rigPrimary}
	got := Clamp(&saved, mons, 0)
	want := Rect{X: 3440 - 500, Y: 100, W: 500, H: 500}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestClamp_OnRemovedSecondMonitorMovesToNearest(t *testing.T) {
	// Saved on the left monitor; the player launches on the primary alone.
	saved := Rect{X: -2100, Y: 350, W: 560, H: 600}
	got := Clamp(&saved, []Rect{rigPrimary}, 0)
	if got.W != 560 || got.H != 600 {
		t.Fatalf("size must be kept: got %+v", got)
	}
	if !inside(got, rigPrimary) {
		t.Fatalf("not inside the only monitor: got %+v", got)
	}
	// Slid in, not recentred: keeps its Y and hugs the edge it came from.
	if got.X != 0 || got.Y != 350 {
		t.Fatalf("expected to hug the left edge at the same Y: got %+v", got)
	}
}

func TestClamp_NegativeCoordinatesLeftMonitorAreValid(t *testing.T) {
	saved := Rect{X: -2560, Y: 77, W: 400, H: 400} // exactly the top-left corner of the left monitor
	if got := Clamp(&saved, rig, rigPrimIdx); got != saved {
		t.Fatalf("negative-origin monitor rejected: got %+v", got)
	}
	// And negative Y on the right monitor.
	saved = Rect{X: 3500, Y: -95, W: 400, H: 400}
	if got := Clamp(&saved, rig, rigPrimIdx); got != saved {
		t.Fatalf("negative-Y monitor rejected: got %+v", got)
	}
}

func TestClamp_LargerThanWorkAreaShrinksToFit(t *testing.T) {
	// Taller than the left monitor (1392) but sits on it. It would fit the
	// right monitor (1560), so this is a shrink, not a fallback.
	saved := Rect{X: -2000, Y: 77, W: 800, H: 1450}
	got := Clamp(&saved, rig, rigPrimIdx)
	if got.H != 1392 || got.W != 800 {
		t.Fatalf("height should shrink to the work area, width kept: got %+v", got)
	}
	if !inside(got, rigLeft) {
		t.Fatalf("not inside the left monitor: got %+v", got)
	}
}

func TestClamp_FitsNoMonitorFallsBackToDefault(t *testing.T) {
	saved := Rect{X: 0, Y: 0, W: 5000, H: 3000}
	got := Clamp(&saved, rig, rigPrimIdx)
	want := Clamp(nil, rig, rigPrimIdx)
	if got != want {
		t.Fatalf("got %+v, want default %+v", got, want)
	}
}

func TestClamp_TieOnIntersectionPrefersNearestCentre(t *testing.T) {
	// Fully off every monitor (zero intersection everywhere), far below the
	// right monitor. Nearest centre is the right monitor's, not the primary's.
	saved := Rect{X: 4400, Y: 3000, W: 500, H: 500}
	got := Clamp(&saved, rig, rigPrimIdx)
	if !inside(got, rigRight) {
		t.Fatalf("expected the right monitor (nearest centre): got %+v", got)
	}
	if got.W != 500 || got.H != 500 {
		t.Fatalf("size must be kept: got %+v", got)
	}
}

func TestClamp_StraddlingTwoMonitorsPicksLargerOverlap(t *testing.T) {
	// 60% on the primary, 40% on the left monitor: primary wins, then it is
	// slid fully onto the primary.
	saved := Rect{X: -200, Y: 300, W: 500, H: 500}
	got := Clamp(&saved, rig, rigPrimIdx)
	if !inside(got, rigPrimary) || got.X != 0 {
		t.Fatalf("expected slid onto primary at X=0: got %+v", got)
	}
}

func TestClamp_NoMonitorsUsesDefaultAtOrigin(t *testing.T) {
	// Defensive: Monitors() failed upstream and the caller passed nothing.
	saved := Rect{X: -2100, Y: 350, W: 560, H: 600}
	got := Clamp(&saved, nil, 0)
	if got != (Rect{X: 0, Y: 0, W: 500, H: 500}) {
		t.Fatalf("got %+v", got)
	}
}
