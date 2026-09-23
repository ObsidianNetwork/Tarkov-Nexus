package mapwindow

import (
	"errors"
	"math"
	"testing"
)

func TestCircleResizeRect_PreservesCenterAndProportions(t *testing.T) {
	// Given round, wide and tall windows, including another monitor's origin.
	tests := []struct {
		name  string
		start Rect
		scale float64
		dpi   uint32
		want  Rect
	}{
		{"round grows", Rect{100, 200, 400, 400}, 1.5, 96, Rect{0, 100, 600, 600}},
		{"landscape grows", Rect{-1000, 100, 800, 400}, 1.5, 96, Rect{-1200, 0, 1200, 600}},
		{"portrait shrinks", Rect{200, -600, 400, 800}, 0.75, 96, Rect{250, -500, 300, 600}},
		{"minimum at 100 percent", Rect{0, 0, 800, 400}, 0.1, 96, Rect{200, 100, 400, 200}},
		{"minimum at 125 percent", Rect{0, 0, 800, 400}, 0.1, 120, Rect{150, 75, 500, 250}},
		{"minimum at 150 percent", Rect{0, 0, 400, 800}, 0.1, 144, Rect{50, 100, 300, 600}},
		{"minimum at 200 percent", Rect{0, 0, 400, 800}, 0.1, 192, Rect{0, 0, 400, 800}},
		{"fractional DPI rounds minimum up", Rect{0, 0, 400, 400}, 0.1, 110, Rect{85, 85, 230, 230}},
		{"one pixel rounding", Rect{100, 100, 401, 601}, 1.2, 96, Rect{60, 40, 481, 721}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When resizing from the initial rectangle.
			got, err := CircleResizeRect(tt.start, tt.scale, tt.dpi)
			// Then scale is absolute and the center is stable to pixel rounding.
			if err != nil || got != tt.want {
				t.Fatalf("CircleResizeRect = (%+v, %v), want %+v", got, err, tt.want)
			}
		})
	}
}

func TestCircleResizeRect_RejectsInvalidRequests(t *testing.T) {
	// Given requests that cannot represent a finite Win32 window rectangle.
	tests := []struct {
		name  string
		start Rect
		scale float64
		dpi   uint32
	}{
		{"zero scale", Rect{0, 0, 400, 400}, 0, 96},
		{"negative scale", Rect{0, 0, 400, 400}, -1, 96},
		{"NaN scale", Rect{0, 0, 400, 400}, math.NaN(), 96},
		{"infinite scale", Rect{0, 0, 400, 400}, math.Inf(1), 96},
		{"zero DPI", Rect{0, 0, 400, 400}, 1, 0},
		{"overflowing DPI", Rect{0, 0, 400, 400}, 1, math.MaxUint32},
		{"zero width", Rect{0, 0, 0, 400}, 1, 96},
		{"negative height", Rect{0, 0, 400, -1}, 1, 96},
		{"overflowing initial width", Rect{0, 0, math.MaxInt32 + 1, 400}, 1, 96},
		{"overflowing initial origin", Rect{math.MaxInt32 + 1, 0, 400, 400}, 1, 96},
		{"underflowing initial origin", Rect{math.MinInt32 - 1, 0, 400, 400}, 1, 96},
		{"overflowing initial right", Rect{math.MaxInt32, 0, 400, 400}, 1, 96},
		{"overflowing result size", Rect{0, 0, 400, 400}, math.MaxFloat64, 96},
		{"underflowing result left", Rect{math.MinInt32, 0, 400, 400}, 2, 96},
		{"overflowing result bottom", Rect{0, math.MaxInt32 - 400, 400, 400}, 2, 96},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When validating the boundary request.
			_, err := CircleResizeRect(tt.start, tt.scale, tt.dpi)
			// Then rejection happens before narrowing values for native APIs.
			if !errors.Is(err, ErrInvalidResize) {
				t.Fatalf("CircleResizeRect error = %v, want ErrInvalidResize", err)
			}
		})
	}
}
