// Package mapwindow owns the party map window's remembered placement (where
// it was, how big, pinned) and appearance (map shape and map opacity),
// restored the next time it opens.
//
// State lives in %APPDATA%\TarkovNexus\mapwindow.json, owned by the map window
// child process alone (never config.json: two writers is a corruption risk).
// Writes sync a temporary file before replacement; anything unreadable loads
// as Defaults() and is rewritten on the next save.
//
// The native adapter talks to user32.dll directly because Wails v2 cannot
// round-trip a window position across monitors: WindowSetPosition is relative
// to the current monitor's work area and ScreenGetAll has no monitor origins.
// All coordinates here are physical pixels in absolute virtual-screen space.
//
// Per-monitor DPI: the first SetWindowPos onto a monitor with a different
// scale factor raises WM_DPICHANGED, which Wails answers by applying Windows'
// suggested (rescaled) rect. PlaceExactly reads back and re-asserts once.
package mapwindow

// Rect is a window rectangle in physical pixels, absolute virtual-screen
// coordinates. Negative X/Y are valid: monitors left of or above the primary
// live there.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

func (r Rect) Right() int   { return r.X + r.W }
func (r Rect) Bottom() int  { return r.Y + r.H }
func (r Rect) Area() int    { return r.W * r.H }
func (r Rect) IsZero() bool { return r.W <= 0 || r.H <= 0 }

// iconicOrigin is where Windows parks a minimised window. Real monitors sit
// left of the primary (negative X/Y) but never this far.
const iconicOrigin = -32000

// IsIconic reports the Win32 minimised sentinel. Negative coordinates on a
// real monitor are valid; this is not that.
func (r Rect) IsIconic() bool {
	return r.X <= iconicOrigin || r.Y <= iconicOrigin
}

// LaunchSize is the Wails create size: saved W×H, or DefaultSize. Position is
// restored later via Win32; Width/Height is the fallback if FindWindow fails.
func LaunchSize(s State) Rect {
	if s.Placement != nil && !s.Placement.IsZero() {
		return Rect{W: s.Placement.W, H: s.Placement.H}
	}
	return DefaultSize
}

// Intersect returns the overlap of r and o, or a zero-area Rect when disjoint.
func (r Rect) Intersect(o Rect) Rect {
	x := max(r.X, o.X)
	y := max(r.Y, o.Y)
	right := min(r.Right(), o.Right())
	bottom := min(r.Bottom(), o.Bottom())
	if right <= x || bottom <= y {
		return Rect{}
	}
	return Rect{X: x, Y: y, W: right - x, H: bottom - y}
}

// SchemaVersion is the mapwindow.json document version. Additive fields do
// not bump it; a breaking change does, and Load treats unknown versions as
// missing.
const SchemaVersion = 1

// DefaultSize is the launch size when nothing is saved: today's 500×500.
var DefaultSize = Rect{W: 500, H: 500}

// State is the persisted document.
type State struct {
	V         int   `json:"v"`
	Placement *Rect `json:"placement,omitempty"` // nil = never saved
	Pinned    bool  `json:"pinned"`
	Shape     Shape `json:"shape,omitempty"`   // "" = square
	Opacity   int   `json:"opacity,omitempty"` // 0 = OpacityDefault
}

// Defaults is the state of a first launch or an unreadable file.
func Defaults() State {
	return State{V: SchemaVersion}
}

// Shape is the mask applied to the map area. A string so more shapes can be
// added without a schema bump.
type Shape string

const (
	ShapeSquare Shape = "square" // default; also what "" means
	ShapeCircle Shape = "circle"
)

// Map opacity bounds, in percent. The pill's slider uses the same numbers.
const (
	OpacityMin     = 20
	OpacityMax     = 100
	OpacityStep    = 5
	OpacityDefault = OpacityMax
)

// Normalize maps every representable value onto a valid one, in place:
// unknown shapes become square; opacity 0 becomes the default, otherwise it
// is clamped to [OpacityMin, OpacityMax] and rounded to OpacityStep. Load
// calls it, so a hand-edited file can never produce an invalid window.
func (s *State) Normalize() {
	s.Shape = s.EffectiveShape()
	s.Opacity = s.EffectiveOpacity()
}

// EffectiveShape is the shape with the default applied.
func (s State) EffectiveShape() Shape {
	switch s.Shape {
	case ShapeSquare, ShapeCircle:
		return s.Shape
	}
	return ShapeSquare
}

// EffectiveOpacity is the opacity with default, clamp and step applied.
func (s State) EffectiveOpacity() int {
	o := s.Opacity
	if o == 0 {
		return OpacityDefault
	}
	o = min(max(o, OpacityMin), OpacityMax)
	// Round half up to the nearest step; Go's integer division truncates.
	return ((o + OpacityStep/2) / OpacityStep) * OpacityStep
}
