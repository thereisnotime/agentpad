package ui

import (
	"errors"
	"sync"

	"github.com/thereisnotime/agentpad/internal/pad"
)

// errDeviceBusy is reported when the device queue is full.
var errDeviceBusy = errors.New("the pad is busy with earlier writes, try again in a moment")

// ledSetter is the part of pad.LEDController the worker uses.
type ledSetter interface {
	Set(pad.LEDSetting) error
	Forget()
}

type jobKind int

const (
	jobUpload jobKind = iota
	jobSetLED
	jobFollow
)

type deviceJob struct {
	kind    jobKind
	mapping []byte
	led     pad.LEDSetting
	done    func(error) // called on the worker goroutine; may be nil
}

// deviceWorker owns all pad I/O. Every write runs on its single goroutine,
// so neither the UI goroutine nor the event receiver ever waits for USB.
// Follow-LED requests are coalesced: only the latest pending one runs.
type deviceWorker struct {
	w   pad.Writer
	led ledSetter

	jobs chan deviceJob
	wake chan struct{} // signals a pending follow job

	mu     sync.Mutex
	follow *deviceJob

	quit     chan struct{}
	finished chan struct{}
	once     sync.Once
}

func newDeviceWorker(w pad.Writer, led ledSetter, queue int) *deviceWorker {
	return &deviceWorker{
		w: w, led: led,
		jobs:     make(chan deviceJob, queue),
		wake:     make(chan struct{}, 1),
		quit:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (d *deviceWorker) start() { go d.run() }

func (d *deviceWorker) run() {
	defer close(d.finished)
	for {
		select {
		case <-d.quit:
			return
		case j := <-d.jobs:
			d.exec(j)
		case <-d.wake:
			d.mu.Lock()
			j := d.follow
			d.follow = nil
			d.mu.Unlock()
			if j != nil {
				d.exec(*j)
			}
		}
	}
}

func (d *deviceWorker) exec(j deviceJob) {
	var err error
	switch j.kind {
	case jobUpload:
		err = d.w.Upload(j.mapping)
	case jobSetLED:
		err = d.w.SetLED(j.led)
		d.led.Forget() // written outside the controller
	case jobFollow:
		err = d.led.Set(j.led)
	}
	if j.done != nil {
		j.done(err)
	}
}

func (d *deviceWorker) submit(j deviceJob) {
	select {
	case d.jobs <- j:
	default:
		if j.done != nil {
			j.done(errDeviceBusy)
		}
	}
}

// Upload queues a mapping upload.
func (d *deviceWorker) Upload(mapping []byte, done func(error)) {
	d.submit(deviceJob{kind: jobUpload, mapping: mapping, done: done})
}

// SetLED queues a manual LED write; the LED controller forgets its state after it.
func (d *deviceWorker) SetLED(s pad.LEDSetting, done func(error)) {
	d.submit(deviceJob{kind: jobSetLED, led: s, done: done})
}

// Follow requests the agent-following LED setting; it never blocks and
// replaces any follow request that has not started yet.
func (d *deviceWorker) Follow(s pad.LEDSetting, done func(error)) {
	d.mu.Lock()
	d.follow = &deviceJob{kind: jobFollow, led: s, done: done}
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default: // a wake-up is already pending and will pick up the latest
	}
}

// Close stops the worker after the job in progress; queued jobs are dropped.
func (d *deviceWorker) Close() {
	d.once.Do(func() {
		close(d.quit)
	})
}

// wait blocks until the worker goroutine has exited (tests).
func (d *deviceWorker) wait() { <-d.finished }
