package debugtap

import (
	"sync"
	"time"
)

// LogLine is one captured formatted log line from the gateway.
type LogLine struct {
	Time time.Time
	Text string // the full formatted line as printed to stdout
}

// LogBuffer is a thread-safe bounded ring buffer of formatted log lines.
// It implements io.Writer so it can be used as a zerolog output tee.
type LogBuffer struct {
	mu   sync.RWMutex
	buf  []LogLine
	head int
	count int
	cap  int
}

// NewLogBuffer creates a LogBuffer with the given capacity.
func NewLogBuffer(capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = 500
	}
	return &LogBuffer{
		buf: make([]LogLine, capacity),
		cap: capacity,
	}
}

// Write implements io.Writer. Each call is treated as one log line.
func (b *LogBuffer) Write(p []byte) (int, error) {
	line := string(p)
	b.mu.Lock()
	b.buf[b.head] = LogLine{Time: time.Now(), Text: line}
	b.head = (b.head + 1) % b.cap
	if b.count < b.cap {
		b.count++
	}
	b.mu.Unlock()
	return len(p), nil
}

// List returns up to limit lines, newest first. limit<=0 returns all.
func (b *LogBuffer) List(limit int) []LogLine {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := b.count
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]LogLine, n)
	for i := 0; i < n; i++ {
		idx := (b.head - 1 - i + b.cap) % b.cap
		out[i] = b.buf[idx]
	}
	return out
}

// Len returns the current number of stored lines.
func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}
