package mapwindow

import (
	"sync"
	"time"
)

// PollerOptions tunes the Poller. Zero values take the defaults; Now and
// After exist so tests can drive time.
type PollerOptions struct {
	Interval time.Duration // default 1s
	Debounce time.Duration // default 500ms
	Now      func() time.Time
	After    func(time.Duration) <-chan time.Time
}

func (o PollerOptions) withDefaults() PollerOptions {
	if o.Interval <= 0 {
		o.Interval = time.Second
	}
	if o.Debounce <= 0 {
		o.Debounce = 500 * time.Millisecond
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.After == nil {
		o.After = time.After
	}
	return o
}

// Poller watches the live window rect and persists changes. It samples on a
// fixed interval rather than hooking WM_MOVE/WM_SIZE, because Wails owns the
// wndproc. Reads while minimised, or whose rect is the Win32 iconic
// sentinel, are ignored: that rect must never overwrite the last visible
// placement.
type Poller struct {
	read  func() (Rect, bool, error)
	write func(Rect) error
	opts  PollerOptions

	startOnce sync.Once
	stopOnce  sync.Once
	started   bool
	ready     chan struct{}
	stop      chan struct{}
	done      chan struct{}
}

// NewPoller wires the sources. read and write are only ever called from the
// poller's own goroutine.
func NewPoller(
	read func() (r Rect, minimised bool, err error),
	write func(Rect) error,
	opts PollerOptions,
) *Poller {
	return &Poller{
		read:  read,
		write: write,
		opts:  opts.withDefaults(),
		ready: make(chan struct{}),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Start captures the initial placement before returning, then polls for changes.
// Calling it more than once is a no-op.
func (p *Poller) Start() {
	p.startOnce.Do(func() {
		p.started = true
		go p.loop()
		<-p.ready
	})
}

// Stop ends the loop and flushes a pending debounced write. Safe to call more
// than once, and safe to call without Start (returns immediately). Blocks
// until the loop has exited.
func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.stop) })
	// startOnce.Do here is a synchronisation point: after it returns, either
	// Start ran first (started=true, loop will close done) or this Do ran
	// first (started=false, and a later Start is a no-op).
	p.startOnce.Do(func() {})
	if p.started {
		<-p.done
	}
}

func (p *Poller) loop() {
	defer close(p.done)

	last, minimised, err := p.read()
	haveLast := err == nil && !minimised && !last.IsZero() && !last.IsIconic()
	var (
		pending  *Rect
		debounce <-chan time.Time
	)
	tick := p.opts.After(p.opts.Interval)
	close(p.ready)

	for {
		select {
		case <-p.stop:
			if pending != nil {
				_ = p.write(*pending)
			}
			return

		case <-debounce:
			debounce = nil
			if pending != nil {
				if err := p.write(*pending); err != nil {
					debounce = p.opts.After(p.opts.Interval)
				} else {
					pending = nil
				}
			}

		case <-tick:
			tick = p.opts.After(p.opts.Interval)
			r, minimised, err := p.read()
			if err != nil || minimised || r.IsZero() || r.IsIconic() {
				continue
			}
			if !haveLast {
				// The rect the window launched with is the baseline, not a change.
				last, haveLast = r, true
				continue
			}
			if r == last {
				continue
			}
			last = r
			pending = &r
			debounce = p.opts.After(p.opts.Debounce)
		}
	}
}
