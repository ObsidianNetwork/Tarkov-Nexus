package mapwindow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

func TestPoller_RetriesFailedWriteWithoutAnotherMove(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given a moved window whose first persistence attempt fails.
		h := newHarness(t)
		h.clk.Advance(time.Second)
		h.mu.Lock()
		h.writeErr = errors.New("temporary file lock")
		h.mu.Unlock()
		want := Rect{X: 700, Y: 80, W: 500, H: 500}
		h.set(want)
		h.clk.Advance(time.Second)
		h.clk.Advance(500 * time.Millisecond)
		if h.writeCount() != 0 {
			t.Fatal("the failed attempt unexpectedly persisted placement")
		}

		// When storage recovers while the window remains in the same position.
		h.mu.Lock()
		h.writeErr = nil
		h.mu.Unlock()
		h.clk.Advance(2 * time.Second)

		// Then the original movement is retried without requiring another move.
		if got := h.lastWrite(); got != want {
			t.Fatalf("placement after storage recovery = %+v, want %+v", got, want)
		}
	})
}

func TestPoller_RetriesStoreWriteAtBoundedIntervals(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Given a real placement store whose destination is blocked by a directory.
		path := filepath.Join(t.TempDir(), "mapwindow.json")
		store := NewStore(path)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		clk := newFakeClock()
		rect := Rect{X: 10, Y: 20, W: 500, H: 500}
		attempts := 0
		var writeErr error
		p := NewPoller(
			func() (Rect, bool, error) { return rect, false, nil },
			func(r Rect) error {
				attempts++
				st := Defaults()
				st.Placement = &r
				writeErr = store.Save(st)
				return writeErr
			},
			PollerOptions{Now: clk.Now, After: clk.After},
		)
		p.Start()
		t.Cleanup(p.Stop)
		synctest.Wait()
		clk.Advance(time.Second)
		want := Rect{X: 700, Y: 80, W: 500, H: 500}
		rect = want
		clk.Advance(time.Second)
		clk.Advance(500 * time.Millisecond)
		if attempts != 1 || writeErr == nil {
			t.Fatalf("expected one failed store write, attempts=%d err=%v", attempts, writeErr)
		}
		clk.Advance(time.Second)
		if attempts != 2 || writeErr == nil {
			t.Fatalf("expected another failed write after one interval, attempts=%d err=%v", attempts, writeErr)
		}

		// When the conflict is removed without another window movement.
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Second - time.Millisecond)
		if attempts != 2 {
			t.Fatalf("retried before the next interval: %d attempts", attempts)
		}
		clk.Advance(time.Millisecond)

		// Then the file contains the placement and successful writes stop retrying.
		st, ok := store.Load()
		if !ok || st.Placement == nil || *st.Placement != want {
			t.Fatalf("placement after file recovery = %+v (ok=%v), want %+v", st.Placement, ok, want)
		}
		clk.Advance(5 * time.Second)
		if attempts != 3 || writeErr != nil {
			t.Fatalf("expected exactly one successful retry, attempts=%d err=%v", attempts, writeErr)
		}
	})
}
