package pad

import (
	"sync"
	"time"
)

// LEDController applies LED modes change-only and at most once per interval.
// A request arriving inside the interval is deferred, and only the latest
// pending mode is applied when the interval elapses.
type LEDController struct {
	w        Writer
	interval time.Duration
	now      func() time.Time
	afterF   func(time.Duration, func()) stopper

	mu        sync.Mutex
	current   LEDSetting // last successfully applied setting, valid when has
	has       bool
	lastWrite time.Time  // time of the last write attempt
	pending   LEDSetting // setting waiting for the timer, valid when timer != nil
	timer     stopper
	closed    bool
}

type stopper interface{ Stop() bool }

// NewLEDController wraps w. Initial mode is unknown (first Set always writes).
func NewLEDController(w Writer, minInterval time.Duration) *LEDController {
	return &LEDController{
		w:        w,
		interval: minInterval,
		now:      time.Now,
		afterF:   func(d time.Duration, f func()) stopper { return time.AfterFunc(d, f) },
	}
}

// Set requests setting n. Returns the error of an immediate write, nil if deferred or unchanged.
func (c *LEDController) Set(n LEDSetting) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if c.timer != nil {
		// A deferred write is already scheduled: just retarget it.
		c.pending = n
		return nil
	}
	if c.has && n == c.current {
		return nil
	}
	if wait := c.interval - c.now().Sub(c.lastWrite); !c.lastWrite.IsZero() && wait > 0 {
		c.pending = n
		c.timer = c.afterF(wait, c.fire)
		return nil
	}
	return c.writeLocked(n)
}

func (c *LEDController) fire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.timer == nil {
		return
	}
	c.timer = nil
	if c.has && c.pending == c.current {
		return
	}
	_ = c.writeLocked(c.pending) // nobody to report to; Current() stays unchanged on error
}

func (c *LEDController) writeLocked(n LEDSetting) error {
	c.lastWrite = c.now()
	if err := c.w.SetLED(n); err != nil {
		return err
	}
	c.current, c.has = n, true
	return nil
}

// Current returns the last successfully applied setting and whether there is one.
func (c *LEDController) Current() (LEDSetting, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current, c.has
}

// Forget clears the remembered setting so the next Set always writes
// (use after writing the LED outside the controller).
func (c *LEDController) Forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.has = false
}

// Close stops any pending deferred write.
func (c *LEDController) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}
