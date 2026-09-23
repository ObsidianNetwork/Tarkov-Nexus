package mapwindow

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestPoller_PersistsMovementBeforeFirstInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given a window which moves immediately after polling starts.
		h := newHarness(t)
		want := Rect{X: 450, Y: 80, W: 720, H: 480}
		h.set(want)

		// When the first interval and debounce pass without another movement.
		h.clk.Advance(time.Second)
		h.clk.Advance(500 * time.Millisecond)

		// Then placement is saved while the process is still running.
		if n := h.writeCount(); n != 1 || h.lastWrite() != want {
			t.Fatalf("first-interval movement persisted %d times: got %+v, want %+v", n, h.lastWrite(), want)
		}
	})
}

func TestPoller_InvalidInitialReadWaitsForVisibleBaseline(t *testing.T) {
	// Given an unavailable, empty or minimised initial window placement.
	tests := []struct {
		name      string
		rect      Rect
		minimised bool
		err       error
	}{
		{"read error", Rect{W: 500, H: 500}, false, errors.New("read failed")},
		{"zero rectangle", Rect{}, false, nil},
		{"minimised", Rect{W: 500, H: 500}, true, nil},
		{"iconic rectangle", Rect{X: -32000, Y: -32000, W: 160, H: 28}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clk := newFakeClock()
				r, minimised, readErr := tt.rect, tt.minimised, tt.err
				var writes []Rect
				p := NewPoller(func() (Rect, bool, error) {
					return r, minimised, readErr
				}, func(rect Rect) error {
					writes = append(writes, rect)
					return nil
				}, PollerOptions{Now: clk.Now, After: clk.After})
				p.Start()
				t.Cleanup(p.Stop)

				// When a valid rectangle becomes readable before the first interval.
				r, minimised, readErr = Rect{X: 10, Y: 20, W: 500, H: 500}, false, nil
				clk.Advance(time.Second)
				clk.Advance(500 * time.Millisecond)

				// Then it establishes the baseline without persisting invalid startup data.
				if len(writes) != 0 {
					t.Fatalf("invalid initial read produced writes: %+v", writes)
				}
			})
		})
	}
}

func TestPoller_StartReturnsAfterBaselineRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given a poller whose reader signals when its baseline is captured.
		read := make(chan struct{}, 1)
		p := NewPoller(func() (Rect, bool, error) {
			read <- struct{}{}
			return Rect{W: 500, H: 500}, false, nil
		}, func(Rect) error { return nil }, PollerOptions{})
		t.Cleanup(p.Stop)

		// When Start returns, before yielding to any other goroutine.
		p.Start()

		// Then the original placement has already been observed.
		select {
		case <-read:
		default:
			t.Fatal("Start returned before capturing the initial window placement")
		}
	})
}
