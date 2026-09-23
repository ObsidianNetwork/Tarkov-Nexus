package mapwindow

import (
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestPoller_NoWriteWhenRectUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		// The initial read is a baseline, not a change worth writing.
		for i := 0; i < 5; i++ {
			h.clk.Advance(time.Second)
		}
		if n := h.writeCount(); n != 0 {
			t.Fatalf("unchanged rect produced %d writes, want 0", n)
		}
	})
}

func TestPoller_WritesOnceAfterDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline

		h.set(Rect{X: 100, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // tick sees the change and arms the debounce
		h.clk.Advance(400 * time.Millisecond)
		if n := h.writeCount(); n != 0 {
			t.Fatalf("wrote before the debounce elapsed: %d", n)
		}
		h.clk.Advance(100 * time.Millisecond) // debounce fires
		if n := h.writeCount(); n != 1 || h.lastWrite().X != 100 {
			t.Fatalf("want exactly one write of X=100, got %+v", h.writes)
		}
		// Idle afterwards: nothing further.
		h.clk.Advance(3 * time.Second)
		if n := h.writeCount(); n != 1 {
			t.Fatalf("idle poller kept writing: %d", n)
		}
	})
}

func TestPoller_ChangesInsideDebounceCoalesceToLatest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Debounce longer than the interval so several ticks land inside it.
		h := newHarnessWith(t, time.Second, 2500*time.Millisecond)
		h.clk.Advance(time.Second) // baseline

		h.set(Rect{X: 100, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // seen, debounce armed
		h.set(Rect{X: 200, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // seen, debounce re-armed
		h.set(Rect{X: 300, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // seen, debounce re-armed
		if n := h.writeCount(); n != 0 {
			t.Fatalf("wrote mid-drag: %+v", h.writes)
		}
		h.clk.Advance(3 * time.Second) // quiet: debounce fires once
		if n := h.writeCount(); n != 1 || h.lastWrite().X != 300 {
			t.Fatalf("want one write of the latest rect (X=300), got %+v", h.writes)
		}
	})
}

func TestPoller_SkipsWhileMinimised(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline

		h.mu.Lock()
		h.minimised = true
		h.rect = Rect{X: -32000, Y: -32000, W: 160, H: 28} // what GetWindowRect reports when iconic
		h.mu.Unlock()
		for i := 0; i < 4; i++ {
			h.clk.Advance(time.Second)
		}
		if n := h.writeCount(); n != 0 {
			t.Fatalf("minimised rect was written %d times", n)
		}
	})
}

func TestPoller_SkipsIconicRectWhenMinimisedFlagFailsOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// IsIconic can return (false, err) or a false-negative; GetWindowRect still
		// reports the -32000 sentinel. That rect must never land in the file.
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline
		h.set(Rect{X: -32000, Y: -32000, W: 160, H: 28})
		for i := 0; i < 4; i++ {
			h.clk.Advance(time.Second)
		}
		if n := h.writeCount(); n != 0 {
			t.Fatalf("iconic sentinel written %d times with minimised=false: %+v", n, h.writes)
		}
	})
}

func TestPoller_StopFlushesPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline
		h.set(Rect{X: 700, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // change seen, debounce armed but not fired
		h.p.Stop()
		if n := h.writeCount(); n != 1 || h.lastWrite().X != 700 {
			t.Fatalf("stop should flush the pending rect once; writes=%+v", h.writes)
		}
	})
}

func TestPoller_StopDoesNotFlushMinimisedRect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline
		h.set(Rect{X: 700, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second) // pending write for 700
		h.mu.Lock()
		h.minimised = true
		h.mu.Unlock()
		h.clk.Advance(time.Second) // tick observes minimised
		h.p.Stop()
		// The pending 700 was a real visible placement; the flush is about *not*
		// writing the iconic rect. 700 landing is fine; the -32000 rect never may.
		for _, w := range h.writes {
			if w.X == -32000 {
				t.Fatalf("iconic rect written: %+v", h.writes)
			}
		}
	})
}

func TestPoller_ReadErrorIsSkippedNotFatal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline
		h.mu.Lock()
		h.readErr = errors.New("GetWindowRect failed")
		h.mu.Unlock()
		h.clk.Advance(time.Second)
		h.clk.Advance(time.Second)
		h.mu.Lock()
		h.readErr = nil
		h.rect = Rect{X: 900, Y: 20, W: 500, H: 500}
		h.mu.Unlock()
		h.clk.Advance(time.Second)
		h.clk.Advance(time.Second)
		if h.lastWrite().X != 900 {
			t.Fatalf("poller did not recover after read error; writes=%+v", h.writes)
		}
	})
}

func TestPoller_StopIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.p.Stop()
		h.p.Stop()
	})
}

func TestPoller_WriteErrorDoesNotStopPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t)
		h.clk.Advance(time.Second) // baseline

		// First write fails (disk full, AV lock). The poller must carry on and
		// the next change must still be persisted.
		h.mu.Lock()
		h.writeErr = errors.New("disk full")
		h.mu.Unlock()
		h.set(Rect{X: 100, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second)
		h.clk.Advance(time.Second) // debounce fires → write fails
		h.mu.Lock()
		h.writeErr = nil
		h.mu.Unlock()
		h.set(Rect{X: 200, Y: 20, W: 500, H: 500})
		h.clk.Advance(time.Second)
		h.clk.Advance(time.Second)
		if h.lastWrite().X != 200 {
			t.Fatalf("poller did not recover after a write error; writes=%+v", h.writes)
		}
	})
}

func TestPoller_StopWithoutStartReturnsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := NewPoller(
			func() (Rect, bool, error) { return Rect{}, false, nil },
			func(Rect) error { return nil },
			PollerOptions{},
		)
		done := make(chan struct{})
		go func() { p.Stop(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Stop blocked forever because Start was never called")
		}
	})
}

func TestPoller_DoubleStartRunsOneLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clk := newFakeClock()
		var mu sync.Mutex
		reads := 0
		p := NewPoller(
			func() (Rect, bool, error) {
				mu.Lock()
				reads++
				mu.Unlock()
				return Rect{X: 1, Y: 1, W: 10, H: 10}, false, nil
			},
			func(Rect) error { return nil },
			PollerOptions{Interval: time.Second, Now: clk.Now, After: clk.After},
		)
		p.Start()
		p.Start() // must be a no-op, not a second goroutine
		synctest.Wait()
		clk.Advance(time.Second)
		mu.Lock()
		n := reads
		mu.Unlock()
		if n != 2 {
			t.Fatalf("baseline plus one tick produced %d reads, want 2", n)
		}
		p.Stop() // must not panic on a double-closed channel
		p.Stop()
	})
}
