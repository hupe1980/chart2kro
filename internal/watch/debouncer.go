package watch

import (
	"log/slog"
	"sync"
	"time"
)

// Debouncer coalesces rapid events into a single callback invocation.
// Only the last event within the configured interval triggers the callback.
type Debouncer struct {
	interval time.Duration
	mu       sync.Mutex
	timer    *time.Timer
	callback func(path string)
	lastPath string
}

// NewDebouncer creates a debouncer that waits for interval of quiet before
// firing callback with the path of the last event.
func NewDebouncer(interval time.Duration, callback func(path string)) *Debouncer {
	return &Debouncer{
		interval: interval,
		callback: callback,
	}
}

// Trigger records an event for the given path. If no further events arrive
// within the debounce interval, the callback fires with the last path seen.
func (d *Debouncer) Trigger(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastPath = path

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.interval, func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("debouncer callback panicked", slog.Any("error", r))
			}
		}()

		d.mu.Lock()
		p := d.lastPath
		d.mu.Unlock()
		d.callback(p)
	})
}

// Stop cancels any pending debounced callback.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}
