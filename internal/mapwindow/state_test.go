package mapwindow

import "testing"

func TestNormalize_EmptyShapeIsSquare(t *testing.T) {
	s := State{V: 1}
	s.Normalize()
	if s.Shape != ShapeSquare {
		t.Fatalf("got %q, want %q", s.Shape, ShapeSquare)
	}
}

func TestNormalize_UnknownShapeIsSquare(t *testing.T) {
	s := State{V: 1, Shape: "hexagon"}
	s.Normalize()
	if s.Shape != ShapeSquare {
		t.Fatalf("got %q, want %q", s.Shape, ShapeSquare)
	}
}

func TestNormalize_ZeroOpacityIsDefault(t *testing.T) {
	s := State{V: 1}
	s.Normalize()
	if s.Opacity != OpacityDefault {
		t.Fatalf("got %d, want %d", s.Opacity, OpacityDefault)
	}
}

func TestNormalize_ClampsBelowMin(t *testing.T) {
	for _, in := range []int{-40, 1, 5, 19} {
		s := State{V: 1, Opacity: in}
		s.Normalize()
		if s.Opacity != OpacityMin {
			t.Fatalf("opacity %d → %d, want %d", in, s.Opacity, OpacityMin)
		}
	}
}

func TestNormalize_ClampsAboveMax(t *testing.T) {
	for _, in := range []int{101, 150, 1 << 20} {
		s := State{V: 1, Opacity: in}
		s.Normalize()
		if s.Opacity != OpacityMax {
			t.Fatalf("opacity %d → %d, want %d", in, s.Opacity, OpacityMax)
		}
	}
}

func TestNormalize_RoundsToStep(t *testing.T) {
	cases := map[int]int{62: 60, 63: 65, 67: 65, 68: 70, 21: 20, 23: 25, 98: 100, 97: 95}
	for in, want := range cases {
		s := State{V: 1, Opacity: in}
		s.Normalize()
		if s.Opacity != want {
			t.Errorf("opacity %d → %d, want %d", in, s.Opacity, want)
		}
	}
}

func TestNormalize_ValidValuesUnchanged(t *testing.T) {
	s := State{V: 1, Shape: ShapeCircle, Opacity: 60, Pinned: true}
	s.Normalize()
	if s.Shape != ShapeCircle || s.Opacity != 60 || !s.Pinned {
		t.Fatalf("valid state mutated: %+v", s)
	}
}

func TestEffectiveAccessorsApplyDefaultsWithoutMutating(t *testing.T) {
	s := State{V: 1}
	if s.EffectiveShape() != ShapeSquare || s.EffectiveOpacity() != OpacityDefault {
		t.Fatalf("effective: shape=%q opacity=%d", s.EffectiveShape(), s.EffectiveOpacity())
	}
	if s.Shape != "" || s.Opacity != 0 {
		t.Fatalf("accessors mutated the state: %+v", s)
	}
}

func TestLaunchSize_NilPlacementIsDefault(t *testing.T) {
	got := LaunchSize(State{V: 1})
	if got != DefaultSize {
		t.Fatalf("got %+v, want %+v", got, DefaultSize)
	}
}

func TestLaunchSize_ZeroPlacementIsDefault(t *testing.T) {
	got := LaunchSize(State{V: 1, Placement: &Rect{X: 100, Y: 100}})
	if got != DefaultSize {
		t.Fatalf("got %+v, want %+v", got, DefaultSize)
	}
}

func TestLaunchSize_UsesSavedSizeNotPosition(t *testing.T) {
	saved := Rect{X: -2100, Y: 350, W: 560, H: 600}
	got := LaunchSize(State{V: 1, Placement: &saved})
	want := Rect{W: 560, H: 600}
	if got != want {
		t.Fatalf("got %+v, want %+v (position must not leak into Wails create size)", got, want)
	}
}

func TestRect_IsIconic(t *testing.T) {
	if (Rect{X: -2100, Y: 350, W: 560, H: 600}).IsIconic() {
		t.Fatal("a left-of-primary monitor is not iconic")
	}
	if !(Rect{X: -32000, Y: -32000, W: 160, H: 28}).IsIconic() {
		t.Fatal("Windows iconic sentinel (-32000,-32000) must be detected")
	}
}
