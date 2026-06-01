// Package debugtap provides a bounded in-memory ring buffer that passively
// records LLM request/response entries from the agent loop for developer debugging.
// It is a read-only observation tap — it never modifies agent behavior.
package debugtap

import (
	"sync"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/providers/protocoltypes"
)

const maxContentBytes = 8 * 1024 // 8 KB per message content

// Entry captures one complete LLM call (request + response).
type Entry struct {
	Seq        uint64
	Timestamp  time.Time
	AgentID    string
	SessionKey string
	Provider   string
	Model      string
	Messages   []protocoltypes.Message
	Tools      []protocoltypes.ToolDefinition
	Response   *protocoltypes.LLMResponse // nil on error
	Err        string
	DurationMS int64
}

// Store is a thread-safe, bounded ring buffer of Entry values.
// When full, the oldest entry is overwritten.
type Store struct {
	mu      sync.RWMutex
	buf     []Entry
	head    int // next write position
	count   int // number of valid entries (≤ cap)
	cap     int
	nextSeq uint64
}

// NewStore creates a Store with the given capacity (must be > 0).
func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = 50
	}
	return &Store{
		buf: make([]Entry, capacity),
		cap: capacity,
	}
}

// Record stores an entry into the ring buffer, overwriting the oldest entry
// when the buffer is full. Messages are deep-copied and content is size-capped.
func (s *Store) Record(e Entry) {
	e.Messages = CloneMessages(e.Messages)
	s.mu.Lock()
	e.Seq = s.nextSeq
	s.nextSeq++
	s.buf[s.head] = e
	s.head = (s.head + 1) % s.cap
	if s.count < s.cap {
		s.count++
	}
	s.mu.Unlock()
}

// List returns up to limit entries, newest first. If limit <= 0, all entries
// are returned.
func (s *Store) List(limit int) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := s.count
	if limit > 0 && limit < n {
		n = limit
	}
	result := make([]Entry, n)
	// head points to the next write slot; step backwards for newest-first
	for i := 0; i < n; i++ {
		idx := (s.head - 1 - i + s.cap) % s.cap
		result[i] = s.buf[idx]
	}
	return result
}

// Get returns the entry with the given sequence number, or false if not found
// (evicted or never recorded).
func (s *Store) Get(seq uint64) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := 0; i < s.count; i++ {
		idx := (s.head - 1 - i + s.cap) % s.cap
		if s.buf[idx].Seq == seq {
			return s.buf[idx], true
		}
	}
	return Entry{}, false
}

// CloneMessages deep-copies a slice of messages and caps each Content field
// to maxContentBytes to bound memory usage.
func CloneMessages(src []protocoltypes.Message) []protocoltypes.Message {
	if src == nil {
		return nil
	}
	out := make([]protocoltypes.Message, len(src))
	for i, m := range src {
		c := m
		if len(c.Content) > maxContentBytes {
			c.Content = c.Content[:maxContentBytes] + "…[truncated]"
		}
		out[i] = c
	}
	return out
}
