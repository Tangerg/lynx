package memory

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// Default and clamp boundaries for [NewMessageWindowStore]. The lower
// bound keeps short windows from completely dropping useful context;
// the upper bound prevents one runaway caller from starving budget /
// memory.
const (
	defaultMessageWindowLimit = 10
	minMessageWindowLimit     = 10
	maxMessageWindowLimit     = 100
)

// ErrListingUnsupported is returned by [MessageWindowStore.Conversations]
// when the wrapped store cannot enumerate conversations.
var ErrListingUnsupported = errors.New("memory: underlying store does not support conversation listing")

var (
	_ Store  = (*MessageWindowStore)(nil)
	_ Lister = (*MessageWindowStore)(nil)
)

// MessageWindowStore wraps another [Store] with a sliding-window
// retention strategy: every Read merges all system messages and keeps
// the most-recent N non-system messages, where N is fixed at
// construction time. Writes pass through unchanged.
type MessageWindowStore struct {
	maximumMessages int
	store           Store
}

// NewMessageWindowStore wraps storage in a sliding-window decorator.
// limit (optional, default 10) is clamped to [10, 100] to avoid
// pathological windows. Wrapping a [MessageWindowStore] is a no-op —
// the existing instance is returned as-is.
//
// Example:
//
//	base := memory.NewInMemoryStore()
//	windowed, err := memory.NewMessageWindowStore(base, 20)
func NewMessageWindowStore(storage Store, limit ...int) (*MessageWindowStore, error) {
	if storage == nil {
		return nil, errors.New("memory.NewMessageWindowStore: storage must not be nil")
	}

	// Don't double-wrap.
	if existing, ok := storage.(*MessageWindowStore); ok {
		return existing, nil
	}

	requested := pkgSlices.AtOr(limit, 0, defaultMessageWindowLimit)
	clamped := max(minMessageWindowLimit, min(maxMessageWindowLimit, requested))

	return &MessageWindowStore{
		maximumMessages: clamped,
		store:           storage,
	}, nil
}

func (m *MessageWindowStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return m.store.Write(ctx, conversationID, messages...)
}

// Replace delegates to the underlying store via [Replace], inheriting its
// atomicity (the wrapped store's [Replacer] when it has one). The sliding
// window is a read-side projection, so a full replace passes straight
// through — the wrapped store holds the authoritative history.
func (m *MessageWindowStore) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	return Replace(ctx, m.store, conversationID, messages...)
}

// Read returns the windowed view: merged system messages first, then
// the most recent non-system messages up to the configured limit.
func (m *MessageWindowStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	all, err := m.store.Read(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return m.applySlidingWindow(all), nil
}

func (m *MessageWindowStore) applySlidingWindow(all []chat.Message) []chat.Message {
	out := make([]chat.Message, 0, m.maximumMessages)
	list := chat.MessageList(all)

	if sys := list.MergeSystem(); sys != nil {
		out = append(out, sys)
	}

	nonSys := list.FilterTypes(chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	remaining := m.maximumMessages - len(out)
	if remaining > 0 && len(nonSys) > 0 {
		start := max(0, len(nonSys)-remaining)
		out = append(out, nonSys[start:]...)
	}
	return out
}

// Conversations forwards to the underlying store when it supports
// listing. The sliding window only affects how a conversation is read
// back, not which conversations exist, so enumeration passes straight
// through. Returns [ErrListingUnsupported] when the wrapped store is not
// a [Lister].
func (m *MessageWindowStore) Conversations(ctx context.Context) ([]string, error) {
	lister, ok := m.store.(Lister)
	if !ok {
		return nil, ErrListingUnsupported
	}
	return lister.Conversations(ctx)
}

func (m *MessageWindowStore) Clear(ctx context.Context, conversationID string) error {
	return m.store.Clear(ctx, conversationID)
}
