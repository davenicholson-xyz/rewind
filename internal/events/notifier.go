// File: internal/events/notifier.go
package events

import (
	"context"
	"fmt"
	"github.com/davenicholson-xyz/rewind/app"
	"github.com/fsnotify/fsnotify"
	"sync"
	"time"
)

// EventCallback defines the function signature for event callbacks
type EventCallback func(event fsnotify.Event)

// EventDebouncer helps reduce duplicate events
type EventDebouncer struct {
	events map[string]time.Time
	mutex  sync.RWMutex
}

func NewEventDebouncer() *EventDebouncer {
	return &EventDebouncer{
		events: make(map[string]time.Time),
	}
}

func (d *EventDebouncer) ShouldProcess(filePath string, eventType string) bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	key := filePath + ":" + eventType
	now := time.Now()

	if lastTime, exists := d.events[key]; exists {
		if now.Sub(lastTime) < 100*time.Millisecond {
			return false
		}
	}

	d.events[key] = now

	// Cleanup old events to prevent memory leaks
	if len(d.events) > 1000 {
		cutoff := now.Add(-5 * time.Second)
		for k, v := range d.events {
			if v.Before(cutoff) {
				delete(d.events, k)
			}
		}
	}

	return true
}

type EventsNotifier struct {
	Notifier  *fsnotify.Watcher
	callback  EventCallback
	debouncer *EventDebouncer
}

func NewEventsNotifier() (*EventsNotifier, error) {
	app.Logger.Debug("Initializing new events notifier")
	notifier, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Could not start event notifier: %w", err)
	}

	en := &EventsNotifier{
		Notifier:  notifier,
		debouncer: NewEventDebouncer(),
	}
	return en, nil
}

func (en *EventsNotifier) AddPath(path string) error {
	app.Logger.WithField("path", path).Debug("Added path to event notifier")
	return en.Notifier.Add(path)
}

// SetCallback sets the callback function for handling events
func (en *EventsNotifier) SetCallback(callback EventCallback) {
	en.callback = callback
}

// Start begins listening for file system events
func (en *EventsNotifier) Start(ctx context.Context) error {
	app.Logger.Info("Starting events notifier")

	for {
		select {
		case event, ok := <-en.Notifier.Events:
			if !ok {
				app.Logger.Debug("Events channel closed")
				return nil
			}
			en.handleEvent(event)

		case err, ok := <-en.Notifier.Errors:
			if !ok {
				app.Logger.Debug("Errors channel closed")
				return nil
			}
			app.Logger.WithError(err).Error("File system watcher error")

		case <-ctx.Done():
			app.Logger.Info("Stopping events notifier due to context cancellation")
			return ctx.Err()
		}
	}
}

// getEventTypeString converts fsnotify.Op to string for debouncing
func (en *EventsNotifier) getEventTypeString(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return "CREATE"
	case op&fsnotify.Write == fsnotify.Write:
		return "WRITE"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "REMOVE"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "RENAME"
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return "CHMOD"
	default:
		return "UNKNOWN"
	}
}

// handleEvent processes individual file system events with debouncing
func (en *EventsNotifier) handleEvent(event fsnotify.Event) {
	logger := app.Logger.WithField("path", event.Name).WithField("op", event.Op.String())

	// Get event type string for debouncing
	eventTypeStr := en.getEventTypeString(event.Op)

	// Check if we should process this event (debouncing)
	if !en.debouncer.ShouldProcess(event.Name, eventTypeStr) {
		logger.Debug("Event debounced - skipping duplicate")
		return
	}

	logger.Debug("Event received and passed debouncing")

	// Log the event type
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		logger.Debug("File created")
	case event.Op&fsnotify.Write == fsnotify.Write:
		logger.Debug("File modified")
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		logger.Debug("File removed")
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		logger.Debug("File renamed")
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		logger.Debug("File permissions changed")
	}

	// Call the callback if set
	if en.callback != nil {
		logger.Debug("Calling event callback")
		en.callback(event)
		logger.Debug("Event callback completed")
	} else {
		logger.Warn("No callback set for event")
	}
}

// Close gracefully shuts down the notifier
func (en *EventsNotifier) Close() error {
	app.Logger.Debug("Closing events notifier")
	return en.Notifier.Close()
}

// RemovePath removes a path from being watched
func (en *EventsNotifier) RemovePath(path string) error {
	app.Logger.WithField("path", path).Debug("Removing path from event notifier")
	return en.Notifier.Remove(path)
}
