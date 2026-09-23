package mapwindow

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeClock drives the poller deterministically. Each Advance releases every
// timer whose deadline has passed, in order.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	return t.ch
}

// Advance moves time forward and fires due timers. It waits between fires until
// the poller has handled each one and is blocked again.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var fire *fakeTimer
		idx := -1
		for i, t := range c.timers {
			if !t.at.After(c.now) {
				fire, idx = t, i
				break
			}
		}
		if fire != nil {
			c.timers = append(c.timers[:idx], c.timers[idx+1:]...)
		}
		c.mu.Unlock()
		if fire == nil {
			return
		}
		fire.ch <- fire.at
		synctest.Wait()
	}
}

// harness wires a poller to fake reads and records writes.
type harness struct {
	clk       *fakeClock
	mu        sync.Mutex
	rect      Rect
	minimised bool
	readErr   error
	writeErr  error
	writes    []Rect
	p         *Poller
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, time.Second, 500*time.Millisecond)
}

func newHarnessWith(t *testing.T, interval, debounce time.Duration) *harness {
	t.Helper()
	h := &harness{clk: newFakeClock(), rect: Rect{X: 10, Y: 20, W: 500, H: 500}}
	h.p = NewPoller(
		func() (Rect, bool, error) {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.rect, h.minimised, h.readErr
		},
		func(r Rect) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.writeErr != nil {
				return h.writeErr
			}
			h.writes = append(h.writes, r)
			return nil
		},
		PollerOptions{Interval: interval, Debounce: debounce, Now: h.clk.Now, After: h.clk.After},
	)
	h.p.Start()
	synctest.Wait()
	t.Cleanup(h.p.Stop)
	return h
}

func (h *harness) set(r Rect) {
	h.mu.Lock()
	h.rect = r
	h.mu.Unlock()
}

func (h *harness) writeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.writes)
}

func (h *harness) lastWrite() Rect {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.writes) == 0 {
		return Rect{}
	}
	return h.writes[len(h.writes)-1]
}
