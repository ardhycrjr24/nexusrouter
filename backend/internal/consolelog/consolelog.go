// Package consolelog provides a thread-safe log buffer that captures request
// pipeline output and streams it to dashboard clients via SSE. It mirrors
// 9router's consoleLogBuffer.js behavior for the Go backend.
package consolelog

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const maxLines = 500

// Event represents a log update. If Clear is true, the client should clear its buffer.
type Event struct {
	Line  string
	Clear bool
}

// Listener receives log events via a buffered channel. The channel-based
// approach ensures that slow listeners never block the publisher.
type Listener struct {
	C chan Event
}

// NewListener creates a listener with a buffered event channel.
func NewListener(bufSize int) *Listener {
	if bufSize <= 0 {
		bufSize = 256 // Log streams can be chatty
	}
	return &Listener{C: make(chan Event, bufSize)}
}

// Buffer is a ring buffer of log lines with pub/sub for SSE streaming.
type Buffer struct {
	mu        sync.RWMutex
	lines     []string
	listeners map[*Listener]struct{}
}

// New creates an empty log buffer.
func New() *Buffer {
	return &Buffer{
		lines:     make([]string, 0, 128),
		listeners: make(map[*Listener]struct{}),
	}
}

// Lines returns a snapshot of all buffered log lines.
func (b *Buffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// Append adds a log line and notifies all listeners.
func (b *Buffer) Append(line string) {
	b.mu.Lock()
	b.lines = append(b.lines, line)
	if len(b.lines) > maxLines {
		// Use copy instead of sub-slicing to release the old backing array for GC.
		n := copy(b.lines, b.lines[len(b.lines)-maxLines:])
		b.lines = b.lines[:n]
	}
	b.mu.Unlock()

	ev := Event{Line: line}
	b.publish(ev)
}

// Clear resets the buffer and notifies all listeners.
func (b *Buffer) Clear() {
	b.mu.Lock()
	b.lines = b.lines[:0]
	b.mu.Unlock()

	b.publish(Event{Clear: true})
}

func (b *Buffer) publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for l := range b.listeners {
		select {
		case l.C <- ev:
		default:
			// Drop event for slow listener — non-blocking.
		}
	}
}

// Subscribe registers a listener. Call Unsubscribe when done.
func (b *Buffer) Subscribe(l *Listener) {
	b.mu.Lock()
	b.listeners[l] = struct{}{}
	b.mu.Unlock()
}

// Unsubscribe removes a listener.
func (b *Buffer) Unsubscribe(l *Listener) {
	b.mu.Lock()
	delete(b.listeners, l)
	b.mu.Unlock()
}

// Logf appends a formatted log line with timestamp and level tag.
// Levels: LOG, INFO, WARN, ERROR, DEBUG.
func (b *Buffer) Logf(level, format string, args ...any) {
	var sb strings.Builder
	sb.Grow(128)
	sb.WriteByte('[')
	sb.WriteString(time.Now().Format("15:04:05.000"))
	sb.WriteString("] [")
	sb.WriteString(level)
	sb.WriteString("] ")
	fmt.Fprintf(&sb, format, args...)
	b.Append(sb.String())
}
