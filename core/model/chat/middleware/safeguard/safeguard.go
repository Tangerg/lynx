package safeguard

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ErrUnsafeContent is returned by [NewMiddleware] when an
// input or output text triggers the configured [Matcher]. Wrap or
// errors.Is against this sentinel to special-case safeguard
// rejections; the underlying error string carries the matched term
// when the matcher chose to expose it.
var ErrUnsafeContent = errors.New("safeguard: unsafe content blocked")

// Matcher is the dependency-inverted predicate that
// [NewMiddleware] consults to decide whether a piece of
// text is allowed. Implementations decide *how* — substring scan,
// compiled regex set, classifier, RPC to a moderation API.
//
// Return ("matched-term", true) to block the call. The matched term
// is folded into [ErrUnsafeContent]; pass "" when the matcher does
// not wish to disclose specifics.
//
// Lynx ships [NewSubstringMatcher] as the stdlib-only default.
type Matcher interface {
	Match(ctx context.Context, text string) (term string, hit bool)
}

// Scope picks which side of the chat exchange the
// middleware inspects.
type Scope int

const (
	// ScopeInput scans the user/system message texts before they
	// reach the model.
	ScopeInput Scope = 1 << iota

	// ScopeOutput scans the assistant text after the model replies
	// (whole response on Call; on each chunk during Stream).
	ScopeOutput

	// ScopeBoth scans inputs AND outputs. Convenience for the
	// common case.
	ScopeBoth = ScopeInput | ScopeOutput
)

func (s Scope) inspectsInput() bool  { return s&ScopeInput != 0 }
func (s Scope) inspectsOutput() bool { return s&ScopeOutput != 0 }

// Options configures [NewMiddleware].
type Options struct {
	// Scope selects which side of the exchange is inspected.
	// Defaults to [ScopeBoth] when zero.
	Scope Scope

	// OnBlock is called when a match triggers a block. The default
	// is no-op; supply your own to log, increment metrics, or push
	// to an audit pipeline. The middleware always rejects with
	// [ErrUnsafeContent] regardless of what this callback does.
	OnBlock func(ctx context.Context, scope Scope, term string)
}

// NewMiddleware returns a (call, stream) middleware pair
// that screens user input and / or assistant output through matcher
// and blocks the request when a hit occurs. Both halves share one
// Matcher so any in-memory state (compiled regex, hash set) is
// allocated once.
//
// Behavior:
//
//   - Input scope: every [chat.SystemMessage].Text and
//     [chat.UserMessage].Text in the request is scanned before the
//     handler runs. A hit short-circuits with [ErrUnsafeContent].
//   - Output scope (Call): the accumulated assistant text from the
//     response is scanned after the handler returns. A hit replaces
//     the response with [ErrUnsafeContent].
//   - Output scope (Stream): each streamed chunk's text delta is
//     scanned as it arrives. The first hit terminates the iteration
//     with [ErrUnsafeContent]; chunks that triggered the hit are
//     still yielded to the consumer so the user sees what was
//     produced before the block.
//
// Passing a nil matcher returns no-op middleware so the call site
// stays stable across feature flags.
//
// Example with default matcher:
//
//	callMW, streamMW := safeguard.NewMiddleware(
//	    safeguard.NewSubstringMatcher([]string{"forbidden", "secret-key"}),
//	    safeguard.Options{Scope: safeguard.ScopeBoth},
//	)
func NewMiddleware(matcher Matcher, opts Options) (chat.CallMiddleware, chat.StreamMiddleware) {
	if matcher == nil {
		return passthroughCall, passthroughStream
	}
	if opts.Scope == 0 {
		opts.Scope = ScopeBoth
	}
	if opts.OnBlock == nil {
		opts.OnBlock = func(context.Context, Scope, string) {}
	}

	mw := &safeguardMiddleware{matcher: matcher, opts: opts}
	return mw.wrapCall, mw.wrapStream
}

type safeguardMiddleware struct {
	matcher Matcher
	opts    Options
}

func passthroughCall(next chat.CallHandler) chat.CallHandler { return next }

func passthroughStream(next chat.StreamHandler) chat.StreamHandler { return next }
