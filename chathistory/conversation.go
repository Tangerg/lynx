package chathistory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidConversationID reports a malformed or unsafe storage key.
var ErrInvalidConversationID = errors.New("chathistory: invalid conversation ID")

type conversationIDContextKey struct{}

// WithConversationID returns a child context carrying the history partition
// key for one model call. An empty ID deliberately shadows and disables an ID
// inherited from a parent context. As with context.WithValue, ctx must not be
// nil.
func WithConversationID(ctx context.Context, conversationID string) context.Context {
	return context.WithValue(ctx, conversationIDContextKey{}, conversationID)
}

// ConversationID returns the ID carried by ctx. Empty values behave as absent
// so middleware can transparently skip history for unbound calls.
func ConversationID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	conversationID, ok := ctx.Value(conversationIDContextKey{}).(string)
	return conversationID, ok && conversationID != ""
}

// ValidateConversationID verifies the partition key required by Store
// operations. IDs are opaque UTF-8 text, but control characters and leading or
// trailing whitespace are rejected because they commonly indicate propagation
// bugs and create invisible or unsafe storage partitions.
func ValidateConversationID(conversationID string) error {
	if conversationID == "" {
		return fmt.Errorf("%w: empty", ErrInvalidConversationID)
	}
	if !utf8.ValidString(conversationID) {
		return fmt.Errorf("%w: invalid UTF-8", ErrInvalidConversationID)
	}
	if strings.TrimSpace(conversationID) != conversationID {
		return fmt.Errorf("%w: leading or trailing whitespace", ErrInvalidConversationID)
	}
	for _, character := range conversationID {
		if unicode.IsControl(character) {
			return fmt.Errorf("%w: contains control character %U", ErrInvalidConversationID, character)
		}
	}
	return nil
}
