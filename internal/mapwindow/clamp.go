package mapwindow

// Clamp decides where the window opens. monitors are work areas (taskbar
// excluded) in physical pixels; primary is the index of the primary monitor.
//
//  1. Nothing saved                       → DefaultSize centred on the primary.
//  2. Pick the monitor with the largest overlap with saved; if none overlap,
//     the one whose centre is nearest.
//  3. Shrink saved to that work area (width and height independently).
//  4. Translate so the rect lies fully inside it.
//  5. If saved is larger than every monitor in either dimension → rule 1.
func Clamp(saved *Rect, monitors []Rect, primary int) Rect {
	if len(monitors) == 0 {
		return Rect{W: DefaultSize.W, H: DefaultSize.H}
	}
	if primary < 0 || primary >= len(monitors) {
		primary = 0
	}
	if saved == nil || saved.IsZero() || !fitsAny(*saved, monitors) {
		return centred(DefaultSize, monitors[primary])
	}

	area := monitors[pickMonitor(*saved, monitors)]
	r := *saved
	r.W = min(r.W, area.W)
	r.H = min(r.H, area.H)
	return slideInside(r, area)
}

func fitsAny(r Rect, monitors []Rect) bool {
	for _, m := range monitors {
		if r.W <= m.W && r.H <= m.H {
			return true
		}
	}
	return false
}

func centred(size, area Rect) Rect {
	return Rect{
		X: area.X + (area.W-size.W)/2,
		Y: area.Y + (area.H-size.H)/2,
		W: size.W,
		H: size.H,
	}
}

// pickMonitor returns the index of the monitor r belongs to: the largest
// intersection wins; with no intersection anywhere, the nearest centre.
func pickMonitor(r Rect, monitors []Rect) int {
	best, bestArea := -1, 0
	for i, m := range monitors {
		if a := r.Intersect(m).Area(); a > bestArea {
			best, bestArea = i, a
		}
	}
	if best >= 0 {
		return best
	}

	rcx, rcy := r.X+r.W/2, r.Y+r.H/2
	bestDist := -1
	for i, m := range monitors {
		dx, dy := rcx-(m.X+m.W/2), rcy-(m.Y+m.H/2)
		if d := dx*dx + dy*dy; bestDist < 0 || d < bestDist {
			best, bestDist = i, d
		}
	}
	return best
}

// slideInside moves r the shortest distance that puts it fully inside area.
// r must already fit (W <= area.W, H <= area.H).
func slideInside(r, area Rect) Rect {
	if r.X < area.X {
		r.X = area.X
	} else if r.Right() > area.Right() {
		r.X = area.Right() - r.W
	}
	if r.Y < area.Y {
		r.Y = area.Y
	} else if r.Bottom() > area.Bottom() {
		r.Y = area.Bottom() - r.H
	}
	return r
}
