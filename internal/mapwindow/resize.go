package mapwindow

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidResize = errors.New("mapwindow: invalid circle resize")

// CircleResizeRect scales physical bounds around their initial center, keeping
// both dimensions proportional and at least 200 logical pixels at the given DPI.
func CircleResizeRect(start Rect, scale float64, dpi uint32) (Rect, error) {
	if !validResizeRect(start) || scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) || dpi == 0 {
		return Rect{}, ErrInvalidResize
	}
	minimum := (uint64(dpi)*200 + 95) / 96
	if minimum > math.MaxInt32 {
		return Rect{}, ErrInvalidResize
	}
	factor := max(scale, float64(minimum)/float64(min(start.W, start.H)))
	w := math.Round(float64(start.W) * factor)
	h := math.Round(float64(start.H) * factor)
	x := math.Round(float64(start.X) + float64(start.W)/2 - w/2)
	y := math.Round(float64(start.Y) + float64(start.H)/2 - h/2)
	if w > math.MaxInt32 || h > math.MaxInt32 || x < math.MinInt32 || y < math.MinInt32 || x+w > math.MaxInt32 || y+h > math.MaxInt32 {
		return Rect{}, ErrInvalidResize
	}
	return Rect{X: int(x), Y: int(y), W: int(w), H: int(h)}, nil
}

func validResizeRect(r Rect) bool {
	return r.W > 0 && r.W <= math.MaxInt32 && r.H > 0 && r.H <= math.MaxInt32 &&
		r.X >= math.MinInt32 && r.X <= math.MaxInt32 && r.Y >= math.MinInt32 && r.Y <= math.MaxInt32 &&
		int64(r.X)+int64(r.W) <= math.MaxInt32 && int64(r.Y)+int64(r.H) <= math.MaxInt32
}

// ResizeCircle applies a circle-edge drag to an already owned native window.
// Persistence remains the placement poller's responsibility.
func ResizeCircle(h HWND, start Rect, scale float64) error {
	dpi, err := WindowDPI(h)
	if err != nil {
		return fmt.Errorf("circle resize: %w", err)
	}
	target, err := CircleResizeRect(start, scale, dpi)
	if err != nil {
		return err
	}
	return PlaceExactly(h, target)
}
