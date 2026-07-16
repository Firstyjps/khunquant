package channels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/bus"
	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

func TestBaseChannelIsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		senderID  string
		want      bool
	}{
		{
			name:      "empty allowlist denies all (default-deny)",
			allowList: nil,
			senderID:  "anyone",
			want:      false,
		},
		{
			name:      "wildcard allowlist allows all",
			allowList: []string{"*"},
			senderID:  "anyone",
			want:      true,
		},
		{
			name:      "compound sender matches numeric allowlist",
			allowList: []string{"123456"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "compound sender matches username allowlist",
			allowList: []string{"@alice"},
			senderID:  "123456|alice",
			want:      true,
		},
		{
			name:      "numeric sender matches legacy compound allowlist",
			allowList: []string{"123456|alice"},
			senderID:  "123456",
			want:      true,
		},
		{
			name:      "non matching sender is denied",
			allowList: []string{"123456"},
			senderID:  "654321|bob",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, tt.allowList)
			if got := ch.IsAllowed(tt.senderID); got != tt.want {
				t.Fatalf("IsAllowed(%q) = %v, want %v", tt.senderID, got, tt.want)
			}
		})
	}
}

func TestShouldRespondInGroup(t *testing.T) {
	tests := []struct {
		name        string
		gt          config.GroupTriggerConfig
		isMentioned bool
		content     string
		wantRespond bool
		wantContent string
	}{
		{
			name:        "no config - permissive default",
			gt:          config.GroupTriggerConfig{},
			isMentioned: false,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "no config - mentioned",
			gt:          config.GroupTriggerConfig{},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "mention_only - not mentioned",
			gt:          config.GroupTriggerConfig{MentionOnly: true},
			isMentioned: false,
			content:     "hello world",
			wantRespond: false,
			wantContent: "hello world",
		},
		{
			name:        "mention_only - mentioned",
			gt:          config.GroupTriggerConfig{MentionOnly: true},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "prefix match",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "/ask hello",
			wantRespond: true,
			wantContent: "hello",
		},
		{
			name:        "prefix no match - not mentioned",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "hello world",
			wantRespond: false,
			wantContent: "hello world",
		},
		{
			name:        "prefix no match - but mentioned",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask"}},
			isMentioned: true,
			content:     "hello world",
			wantRespond: true,
			wantContent: "hello world",
		},
		{
			name:        "multiple prefixes - second matches",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask", "/bot"}},
			isMentioned: false,
			content:     "/bot help me",
			wantRespond: true,
			wantContent: "help me",
		},
		{
			name:        "mention_only with prefixes - mentioned overrides",
			gt:          config.GroupTriggerConfig{MentionOnly: true, Prefixes: []string{"/ask"}},
			isMentioned: true,
			content:     "hello",
			wantRespond: true,
			wantContent: "hello",
		},
		{
			name:        "mention_only with prefixes - not mentioned, no prefix",
			gt:          config.GroupTriggerConfig{MentionOnly: true, Prefixes: []string{"/ask"}},
			isMentioned: false,
			content:     "hello",
			wantRespond: false,
			wantContent: "hello",
		},
		{
			name:        "empty prefix in list is skipped",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"", "/ask"}},
			isMentioned: false,
			content:     "/ask test",
			wantRespond: true,
			wantContent: "test",
		},
		{
			name:        "prefix strips leading whitespace after prefix",
			gt:          config.GroupTriggerConfig{Prefixes: []string{"/ask "}},
			isMentioned: false,
			content:     "/ask hello",
			wantRespond: true,
			wantContent: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, nil, WithGroupTrigger(tt.gt))
			gotRespond, gotContent := ch.ShouldRespondInGroup(tt.isMentioned, tt.content)
			if gotRespond != tt.wantRespond {
				t.Errorf("ShouldRespondInGroup() respond = %v, want %v", gotRespond, tt.wantRespond)
			}
			if gotContent != tt.wantContent {
				t.Errorf("ShouldRespondInGroup() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}

func TestIsAllowedSender(t *testing.T) {
	tests := []struct {
		name      string
		allowList []string
		sender    bus.SenderInfo
		want      bool
	}{
		{
			name:      "empty allowlist denies all (default-deny)",
			allowList: nil,
			sender:    bus.SenderInfo{PlatformID: "anyone"},
			want:      false,
		},
		{
			name:      "wildcard allowlist allows all",
			allowList: []string{"*"},
			sender:    bus.SenderInfo{PlatformID: "anyone"},
			want:      true,
		},
		{
			name:      "numeric ID matches PlatformID",
			allowList: []string{"123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: true,
		},
		{
			name:      "canonical format matches",
			allowList: []string{"telegram:123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: true,
		},
		{
			name:      "canonical format wrong platform",
			allowList: []string{"discord:123456"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: false,
		},
		{
			name:      "@username matches",
			allowList: []string{"@alice"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
				Username:    "alice",
			},
			want: true,
		},
		{
			name:      "compound id|username matches by ID",
			allowList: []string{"123456|alice"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
				Username:    "alice",
			},
			want: true,
		},
		{
			name:      "non matching sender denied",
			allowList: []string{"654321"},
			sender: bus.SenderInfo{
				Platform:    "telegram",
				PlatformID:  "123456",
				CanonicalID: "telegram:123456",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := NewBaseChannel("test", nil, nil, tt.allowList)
			if got := ch.IsAllowedSender(tt.sender); got != tt.want {
				t.Fatalf("IsAllowedSender(%+v) = %v, want %v", tt.sender, got, tt.want)
			}
		})
	}
}

func TestWithMaxMessageLength(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil, WithMaxMessageLength(100))
	if got := ch.MaxMessageLength(); got != 100 {
		t.Fatalf("MaxMessageLength() = %d, want 100", got)
	}
}

func TestWithReasoningChannelID(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil, WithReasoningChannelID("reasoning-channel"))
	if got := ch.ReasoningChannelID(); got != "reasoning-channel" {
		t.Fatalf("ReasoningChannelID() = %s, want reasoning-channel", got)
	}
}

func TestBaseChannel_Name(t *testing.T) {
	ch := NewBaseChannel("my-channel", nil, nil, nil)
	if got := ch.Name(); got != "my-channel" {
		t.Fatalf("Name() = %s, want my-channel", got)
	}
}

func TestBaseChannel_IsRunning(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	if ch.IsRunning() {
		t.Fatal("expected IsRunning() = false initially")
	}

	ch.SetRunning(true)
	if !ch.IsRunning() {
		t.Fatal("expected IsRunning() = true after SetRunning(true)")
	}

	ch.SetRunning(false)
	if ch.IsRunning() {
		t.Fatal("expected IsRunning() = false after SetRunning(false)")
	}
}

func TestBaseChannel_MediaStore(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	if ch.GetMediaStore() != nil {
		t.Fatal("expected GetMediaStore() = nil initially")
	}

	// Verify that SetMediaStore works (we don't verify the type since it's an interface)
	// Just verify the setter doesn't panic
	ch.SetMediaStore(nil)
	if ch.GetMediaStore() != nil {
		t.Fatal("expected GetMediaStore() = nil after setting nil")
	}
}

func TestBaseChannel_PlaceholderRecorder(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	if ch.GetPlaceholderRecorder() != nil {
		t.Fatal("expected GetPlaceholderRecorder() = nil initially")
	}

	// Create a mock PlaceholderRecorder that satisfies the interface
	mockRec := &mockPlaceholderRecorder{}
	ch.SetPlaceholderRecorder(mockRec)
	retrieved := ch.GetPlaceholderRecorder()
	if retrieved == nil {
		t.Fatal("expected GetPlaceholderRecorder() to return a recorder")
	}
}

// mockPlaceholderRecorder satisfies the PlaceholderRecorder interface
type mockPlaceholderRecorder struct{}

func (m *mockPlaceholderRecorder) RecordPlaceholder(channel, chatID, placeholderID string) {}
func (m *mockPlaceholderRecorder) RecordTypingStop(channel, chatID string, stop func())    {}
func (m *mockPlaceholderRecorder) RecordReactionUndo(channel, chatID string, undo func()) {}

func TestBaseChannel_SetOwner(t *testing.T) {
	ch := NewBaseChannel("test", nil, nil, nil)
	mockOwner := &mockChannelForOwner{}
	ch.SetOwner(mockOwner)
	// Verify by checking internal state (owner field is set)
	// We can't directly test ch.owner since it's unexported, but the setter doesn't panic
}

// mockChannelForOwner is a simple mock for testing SetOwner
type mockChannelForOwner struct{}

func (m *mockChannelForOwner) Name() string                                        { return "mock" }
func (m *mockChannelForOwner) Start(_ context.Context) error                       { return nil }
func (m *mockChannelForOwner) Stop(_ context.Context) error                        { return nil }
func (m *mockChannelForOwner) Send(_ context.Context, _ bus.OutboundMessage) error { return nil }
func (m *mockChannelForOwner) IsRunning() bool                                     { return false }
func (m *mockChannelForOwner) IsAllowed(senderID string) bool                      { return true }
func (m *mockChannelForOwner) IsAllowedSender(sender bus.SenderInfo) bool          { return true }
func (m *mockChannelForOwner) ReasoningChannelID() string                          { return "" }

func TestBaseChannel_HandleMessage_AllowCheck(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"allowed-user"})

	ctx := context.Background()
	var published bus.InboundMessage
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	// Call HandleMessage with allowed user
	ch.HandleMessage(ctx, bus.Peer{}, "msg1", "allowed-user", "chat1", "hello", nil, nil)

	select {
	case published = <-publishedChan:
		if published.Content != "hello" {
			t.Fatalf("expected content 'hello', got %q", published.Content)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}

	// Call HandleMessage with disallowed user (should be ignored)
	ch.HandleMessage(ctx, bus.Peer{}, "msg2", "disallowed-user", "chat1", "world", nil, nil)

	select {
	case <-publishedChan:
		t.Fatal("expected HandleMessage to reject disallowed user")
	case <-time.After(100 * time.Millisecond):
		// Expected: no message published
	}
}

func TestBuildMediaScope(t *testing.T) {
	tests := []struct {
		channel   string
		chatID    string
		messageID string
		wantScope string
	}{
		{
			channel:   "telegram",
			chatID:    "chat123",
			messageID: "msg456",
			wantScope: "telegram:chat123:msg456",
		},
		{
			channel:   "discord",
			chatID:    "ch789",
			messageID: "",
			// wantScope contains a generated ID, so we just check the prefix
		},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			scope := BuildMediaScope(tt.channel, tt.chatID, tt.messageID)
			if tt.messageID != "" && scope != tt.wantScope {
				t.Fatalf("BuildMediaScope(%q, %q, %q) = %q, want %q",
					tt.channel, tt.chatID, tt.messageID, scope, tt.wantScope)
			}
			// When messageID is empty, verify format
			if tt.messageID == "" && !strings.HasPrefix(scope, tt.channel+":"+tt.chatID+":") {
				t.Fatalf("BuildMediaScope(%q, %q, %q) = %q, expected format channel:chatID:generatedID",
					tt.channel, tt.chatID, tt.messageID, scope)
			}
		})
	}
}

func TestHandleMessage_WithSenderInfo(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"*"})

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	senderInfo := bus.SenderInfo{
		Platform:    "telegram",
		PlatformID:  "12345",
		CanonicalID: "telegram:12345",
		Username:    "alice",
	}

	ch.HandleMessage(ctx, bus.Peer{}, "msg1", "12345", "chat1", "hello", nil, nil, senderInfo)

	select {
	case published := <-publishedChan:
		if published.SenderID != "telegram:12345" {
			t.Fatalf("expected SenderID 'telegram:12345', got %q", published.SenderID)
		}
		if published.Sender.Username != "alice" {
			t.Fatalf("expected Sender.Username 'alice', got %q", published.Sender.Username)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_MediaScope(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"*"})

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	ch.HandleMessage(ctx, bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case published := <-publishedChan:
		expectedScope := "test:chat1:msg1"
		if published.MediaScope != expectedScope {
			t.Fatalf("expected MediaScope %q, got %q", expectedScope, published.MediaScope)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_MetadataPreserved(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"*"})

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	metadata := map[string]string{"key": "value"}
	peer := bus.Peer{Kind: "direct", ID: "peer1"}

	ch.HandleMessage(ctx, peer, "msg1", "user1", "chat1", "hello", nil, metadata)

	select {
	case published := <-publishedChan:
		if published.Metadata["key"] != "value" {
			t.Fatalf("expected metadata preserved, got %v", published.Metadata)
		}
		if published.Peer.ID != "peer1" {
			t.Fatalf("expected peer preserved, got %+v", published.Peer)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_Disallowed(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"allowed-user"})

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	ch.HandleMessage(ctx, bus.Peer{}, "msg1", "disallowed-user", "chat1", "hello", nil, nil)

	select {
	case <-publishedChan:
		t.Fatal("expected HandleMessage to reject disallowed user")
	case <-time.After(100 * time.Millisecond):
		// Expected: no message published
	}
}

func TestHandleMessage_DisallowedViaCanonicalID(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	ch := NewBaseChannel("test", nil, mb, []string{"telegram:allowed"})

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	disallowedSender := bus.SenderInfo{
		Platform:    "telegram",
		CanonicalID: "telegram:disallowed",
	}

	ch.HandleMessage(ctx, bus.Peer{}, "msg1", "disallowed", "chat1", "hello", nil, nil, disallowedSender)

	select {
	case <-publishedChan:
		t.Fatal("expected HandleMessage to reject disallowed sender")
	case <-time.After(100 * time.Millisecond):
		// Expected: no message published
	}
}

func TestHandleMessage_WithTypingCapable(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	// Create a channel that supports typing
	typingCh := &mockTypingChannel{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
	}

	baseCh := NewBaseChannel("test", nil, mb, []string{"*"})
	baseCh.SetOwner(typingCh)
	mockRec := &mockPlaceholderRecorder{}
	baseCh.SetPlaceholderRecorder(mockRec)

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	baseCh.HandleMessage(ctx, bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case published := <-publishedChan:
		if published.Content != "hello" {
			t.Fatalf("expected content 'hello', got %q", published.Content)
		}
		// Verify typing was started (check the typingCh mock)
		if typingCh.typingStarted != 1 {
			t.Fatalf("expected typing to be started once, got %d", typingCh.typingStarted)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_WithReactionCapable(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	// Create a channel that supports reactions
	reactionCh := &mockReactionChannel{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
	}

	baseCh := NewBaseChannel("test", nil, mb, []string{"*"})
	baseCh.SetOwner(reactionCh)
	mockRec := &mockPlaceholderRecorder{}
	baseCh.SetPlaceholderRecorder(mockRec)

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	baseCh.HandleMessage(ctx, bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case published := <-publishedChan:
		if published.Content != "hello" {
			t.Fatalf("expected content 'hello', got %q", published.Content)
		}
		// Verify reaction was triggered
		if reactionCh.reactionTriggered != 1 {
			t.Fatalf("expected reaction to be triggered once, got %d", reactionCh.reactionTriggered)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_WithPlaceholderCapableNoAudio(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	// Create a channel that supports placeholders
	placeholderCh := &mockPlaceholderCapableChannel{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
	}

	baseCh := NewBaseChannel("test", nil, mb, []string{"*"})
	baseCh.SetOwner(placeholderCh)
	mockRec := &mockPlaceholderRecorder{}
	baseCh.SetPlaceholderRecorder(mockRec)

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	// Send message without audio annotation
	baseCh.HandleMessage(ctx, bus.Peer{}, "msg1", "user1", "chat1", "hello", nil, nil)

	select {
	case published := <-publishedChan:
		if published.Content != "hello" {
			t.Fatalf("expected content 'hello', got %q", published.Content)
		}
		// Verify placeholder was sent
		if placeholderCh.placeholderSent != 1 {
			t.Fatalf("expected placeholder to be sent once, got %d", placeholderCh.placeholderSent)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

func TestHandleMessage_WithPlaceholderCapableAudioAnnotation(t *testing.T) {
	mb := bus.NewMessageBus()
	defer mb.Close()

	// Create a channel that supports placeholders
	placeholderCh := &mockPlaceholderCapableChannel{
		mockChannel: mockChannel{
			sendFn: func(_ context.Context, _ bus.OutboundMessage) error {
				return nil
			},
		},
	}

	baseCh := NewBaseChannel("test", nil, mb, []string{"*"})
	baseCh.SetOwner(placeholderCh)
	mockRec := &mockPlaceholderRecorder{}
	baseCh.SetPlaceholderRecorder(mockRec)

	ctx := context.Background()
	publishedChan := make(chan bus.InboundMessage, 1)

	go func() {
		msg, ok := <-mb.InboundChan()
		if ok {
			publishedChan <- msg
		}
	}()

	// Send message with audio annotation - placeholder should NOT be sent
	baseCh.HandleMessage(ctx, bus.Peer{}, "msg1", "user1", "chat1", "[voice] hello", nil, nil)

	select {
	case published := <-publishedChan:
		if published.Content != "[voice] hello" {
			t.Fatalf("expected content '[voice] hello', got %q", published.Content)
		}
		// Verify placeholder was NOT sent (audio messages skip placeholder)
		if placeholderCh.placeholderSent != 0 {
			t.Fatalf("expected placeholder NOT to be sent for audio message, got %d", placeholderCh.placeholderSent)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("expected HandleMessage to publish inbound message")
	}
}

// Mock implementations for testing capability interfaces

type mockTypingChannel struct {
	mockChannel
	typingStarted int
}

func (m *mockTypingChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	m.typingStarted++
	return func() {}, nil
}

type mockReactionChannel struct {
	mockChannel
	reactionTriggered int
}

func (m *mockReactionChannel) ReactToMessage(ctx context.Context, chatID, messageID string) (func(), error) {
	m.reactionTriggered++
	return func() {}, nil
}

type mockPlaceholderCapableChannel struct {
	mockChannel
	placeholderSent int
}

func (m *mockPlaceholderCapableChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	m.placeholderSent++
	return "placeholder-123", nil
}
