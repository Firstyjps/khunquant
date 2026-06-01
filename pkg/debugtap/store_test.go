package debugtap_test

import (
	"sync"
	"testing"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/debugtap"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/protocoltypes"
)

// TestNewStore_DefaultCapacity ensures NewStore(0) uses a sensible default.
func TestNewStore_DefaultCapacity(t *testing.T) {
	s := debugtap.NewStore(0)
	if s == nil {
		t.Fatal("NewStore(0) returned nil")
	}
	// List should not panic and should have space for at least one entry.
	entries := s.List(0)
	if entries == nil {
		t.Fatal("List(0) returned nil slice")
	}
	// Record one entry and verify it's stored.
	s.Record(debugtap.Entry{
		AgentID:   "test",
		Timestamp: time.Now(),
		Messages: []protocoltypes.Message{
			{Role: "user", Content: "hello"},
		},
	})
	entries = s.List(0)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after Record, got %d", len(entries))
	}
}

// TestRecord_List_Basic records 3 entries and verifies List returns them newest first.
func TestRecord_List_Basic(t *testing.T) {
	s := debugtap.NewStore(10)
	now := time.Now()

	entries := []debugtap.Entry{
		{
			AgentID:   "test",
			Timestamp: now,
			Messages: []protocoltypes.Message{
				{Role: "user", Content: "first"},
			},
		},
		{
			AgentID:   "test",
			Timestamp: now.Add(time.Second),
			Messages: []protocoltypes.Message{
				{Role: "user", Content: "second"},
			},
		},
		{
			AgentID:   "test",
			Timestamp: now.Add(2 * time.Second),
			Messages: []protocoltypes.Message{
				{Role: "user", Content: "third"},
			},
		},
	}

	for _, e := range entries {
		s.Record(e)
	}

	listed := s.List(0)
	if len(listed) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(listed))
	}

	// Verify newest first (highest Seq first)
	if listed[0].Seq <= listed[1].Seq || listed[1].Seq <= listed[2].Seq {
		t.Errorf("expected descending Seq order, got [%d, %d, %d]",
			listed[0].Seq, listed[1].Seq, listed[2].Seq)
	}

	if listed[0].Messages[0].Content != "third" {
		t.Errorf("expected newest entry to be 'third', got %q", listed[0].Messages[0].Content)
	}
}

// TestRecord_RingWrap verifies that oldest entries are evicted when capacity is exceeded.
func TestRecord_RingWrap(t *testing.T) {
	s := debugtap.NewStore(3)

	// Record 5 entries into a capacity-3 store
	for i := 0; i < 5; i++ {
		s.Record(debugtap.Entry{
			AgentID:   "test",
			Timestamp: time.Now(),
			Messages: []protocoltypes.Message{
				{Role: "user", Content: "msg-" + string(rune('0'+i))},
			},
		})
	}

	listed := s.List(0)
	if len(listed) != 3 {
		t.Fatalf("expected 3 entries (capacity), got %d", len(listed))
	}

	// Verify we have the newest 3 (indices 2, 3, 4 → Seq 2, 3, 4)
	if listed[0].Seq != 4 || listed[1].Seq != 3 || listed[2].Seq != 2 {
		t.Errorf("expected Seq [4, 3, 2], got [%d, %d, %d]",
			listed[0].Seq, listed[1].Seq, listed[2].Seq)
	}

	// Verify oldest entries (0, 1 → Seq 0, 1) are evicted
	_, found0 := s.Get(0)
	_, found1 := s.Get(1)
	if found0 || found1 {
		t.Error("expected Seq 0 and 1 to be evicted, but they were found")
	}
}

// TestGet_Found retrieves an entry by Seq after recording it.
func TestGet_Found(t *testing.T) {
	s := debugtap.NewStore(5)

	entry := debugtap.Entry{
		AgentID:   "test",
		Timestamp: time.Now(),
		Messages: []protocoltypes.Message{
			{Role: "user", Content: "hello"},
		},
	}
	s.Record(entry)

	// The recorded entry should have Seq 0 (first entry)
	retrieved, found := s.Get(0)
	if !found {
		t.Fatal("expected to find Seq 0, but it was not found")
	}

	if retrieved.Messages[0].Content != "hello" {
		t.Errorf("expected content 'hello', got %q", retrieved.Messages[0].Content)
	}
}

// TestGet_NotFound returns false for a Seq that was never recorded or was evicted.
func TestGet_NotFound(t *testing.T) {
	s := debugtap.NewStore(3)

	// Record 5 entries, evicting Seq 0 and 1
	for i := 0; i < 5; i++ {
		s.Record(debugtap.Entry{
			AgentID:   "test",
			Timestamp: time.Now(),
			Messages: []protocoltypes.Message{
				{Role: "user", Content: "msg"},
			},
		})
	}

	// Seq 0 and 1 should be evicted
	_, found0 := s.Get(0)
	if found0 {
		t.Error("expected Seq 0 to be evicted")
	}

	// Seq 999 was never recorded
	_, found999 := s.Get(999)
	if found999 {
		t.Error("expected Seq 999 to not be found")
	}
}

// TestRecord_DeepCopy verifies that messages are deep-copied on Record.
func TestRecord_DeepCopy(t *testing.T) {
	s := debugtap.NewStore(5)

	messages := []protocoltypes.Message{
		{Role: "user", Content: "original-content"},
	}

	s.Record(debugtap.Entry{
		AgentID:   "test",
		Timestamp: time.Now(),
		Messages:  messages,
	})

	// Mutate the original message
	messages[0].Content = "mutated-content"

	// Verify the stored entry still has the original content
	listed := s.List(0)
	if len(listed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(listed))
	}

	if listed[0].Messages[0].Content != "original-content" {
		t.Errorf("expected 'original-content' (deep copy), got %q",
			listed[0].Messages[0].Content)
	}
}

// TestRecord_ContentSizeCap verifies that message content is capped at 8KB + "…[truncated]".
func TestRecord_ContentSizeCap(t *testing.T) {
	s := debugtap.NewStore(5)

	// Create a message with 10KB of content (exceeds 8KB limit)
	largeContent := make([]byte, 10000)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	s.Record(debugtap.Entry{
		AgentID:   "test",
		Timestamp: time.Now(),
		Messages: []protocoltypes.Message{
			{Role: "user", Content: string(largeContent)},
		},
	})

	listed := s.List(0)
	if len(listed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(listed))
	}

	stored := listed[0].Messages[0].Content
	maxBytes := 8192 + len("…[truncated]")

	if len(stored) > maxBytes {
		t.Errorf("expected content <= %d bytes, got %d", maxBytes, len(stored))
	}

	// Verify truncation marker is present
	if !contains(stored, "…[truncated]") {
		t.Errorf("expected truncation marker '…[truncated]', not found in stored content")
	}
}

// TestStore_Concurrent verifies thread safety with concurrent Record and List operations.
func TestStore_Concurrent(t *testing.T) {
	s := debugtap.NewStore(100)
	var wg sync.WaitGroup

	// 10 goroutines, each recording 20 entries
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				s.Record(debugtap.Entry{
					AgentID:   "test",
					Timestamp: time.Now(),
					Messages: []protocoltypes.Message{
						{Role: "user", Content: "msg"},
					},
				})
			}
		}(g)
	}

	// 5 goroutines, each calling List(10) repeatedly
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = s.List(10)
			}
		}()
	}

	wg.Wait()

	// All 200 entries should be stored (10 goroutines * 20 entries)
	listed := s.List(0)
	if len(listed) != 100 {
		// Store capacity is 100, so we expect at most 100 entries
		if len(listed) > 100 {
			t.Errorf("expected at most 100 entries, got %d", len(listed))
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && s[len(s)-len(substr):] == substr) || s[:len(substr)] == substr)
}
