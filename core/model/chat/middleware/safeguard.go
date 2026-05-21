package middleware

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ErrUnsafeContent is returned by [NewSafeguardMiddleware] when an
// input or output text triggers the configured [Matcher]. Wrap or
// errors.Is against this sentinel to special-case safeguard
// rejections; the underlying error string carries the matched term
// when the matcher chose to expose it.
var ErrUnsafeContent = errors.New("chat.middleware: unsafe content blocked")

// Matcher is the dependency-inverted predicate that
// [NewSafeguardMiddleware] consults to decide whether a piece of
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

// SafeguardScope picks which side of the chat exchange the
// middleware inspects.
type SafeguardScope int

const (
	// ScopeInput scans the user/system message texts before they
	// reach the model.
	ScopeInput SafeguardScope = 1 << iota

	// ScopeOutput scans the assistant text after the model replies
	// (whole response on Call; on each chunk during Stream).
	ScopeOutput

	// ScopeBoth scans inputs AND outputs. Convenience for the
	// common case.
	ScopeBoth = ScopeInput | ScopeOutput
)

func (s SafeguardScope) inspectsInput() bool  { return s&ScopeInput != 0 }
func (s SafeguardScope) inspectsOutput() bool { return s&ScopeOutput != 0 }

// SafeguardOptions configures [NewSafeguardMiddleware].
type SafeguardOptions struct {
	// Scope selects which side of the exchange is inspected.
	// Defaults to [ScopeBoth] when zero.
	Scope SafeguardScope

	// OnBlock is called when a match triggers a block. The default
	// is no-op; supply your own to log, increment metrics, or push
	// to an audit pipeline. The middleware always rejects with
	// [ErrUnsafeContent] regardless of what this callback does.
	OnBlock func(ctx context.Context, scope SafeguardScope, term string)
}

// NewSafeguardMiddleware returns a (call, stream) middleware pair
// that screens user input and / or assistant output through matcher
// and blocks the request when a hit occurs. Both halves share one
// Matcher so any in-memory state (compiled regex, hash set) is
// allocated once.
//
// Behaviour:
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
//	callMW, streamMW := middleware.NewSafeguardMiddleware(
//	    middleware.NewSubstringMatcher([]string{"forbidden", "secret-key"}, true),
//	    middleware.SafeguardOptions{Scope: middleware.ScopeBoth},
//	)
func NewSafeguardMiddleware(matcher Matcher, opts SafeguardOptions) (chat.CallMiddleware, chat.StreamMiddleware) {
	if matcher == nil {
		return passthroughCall, passthroughStream
	}
	if opts.Scope == 0 {
		opts.Scope = ScopeBoth
	}
	if opts.OnBlock == nil {
		opts.OnBlock = func(context.Context, SafeguardScope, string) {}
	}

	mw := &safeguardMiddleware{matcher: matcher, opts: opts}
	return mw.wrapCall, mw.wrapStream
}

type safeguardMiddleware struct {
	matcher Matcher
	opts    SafeguardOptions
}

func (m *safeguardMiddleware) wrapCall(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		if m.opts.Scope.inspectsInput() {
			if term, hit := m.scanInputs(ctx, req); hit {
				m.opts.OnBlock(ctx, ScopeInput, term)
				return nil, blockError(ScopeInput, term)
			}
		}

		resp, err := next.Call(ctx, req)
		if err != nil {
			return resp, err
		}

		if m.opts.Scope.inspectsOutput() && resp != nil && resp.Result != nil && resp.Result.AssistantMessage != nil {
			text := resp.Result.AssistantMessage.JoinedText()
			if text != "" {
				if term, hit := m.matcher.Match(ctx, text); hit {
					m.opts.OnBlock(ctx, ScopeOutput, term)
					return nil, blockError(ScopeOutput, term)
				}
			}
		}
		return resp, nil
	})
}

func (m *safeguardMiddleware) wrapStream(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			if m.opts.Scope.inspectsInput() {
				if term, hit := m.scanInputs(ctx, req); hit {
					m.opts.OnBlock(ctx, ScopeInput, term)
					yield(nil, blockError(ScopeInput, term))
					return
				}
			}

			for resp, err := range next.Stream(ctx, req) {
				if err != nil {
					if !yield(resp, err) {
						return
					}
					continue
				}

				if !yield(resp, nil) {
					return
				}

				if m.opts.Scope.inspectsOutput() && resp != nil && resp.Result != nil && resp.Result.AssistantMessage != nil {
					text := resp.Result.AssistantMessage.JoinedText()
					if text != "" {
						if term, hit := m.matcher.Match(ctx, text); hit {
							m.opts.OnBlock(ctx, ScopeOutput, term)
							yield(nil, blockError(ScopeOutput, term))
							return
						}
					}
				}
			}
		}
	})
}

// scanInputs walks system / user messages and runs each non-empty
// text through the matcher. ToolMessages and AssistantMessages from
// prior turns are skipped — they're not user-authored.
func (m *safeguardMiddleware) scanInputs(ctx context.Context, req *chat.Request) (string, bool) {
	if req == nil {
		return "", false
	}
	for _, msg := range req.Messages {
		var text string
		switch v := msg.(type) {
		case *chat.UserMessage:
			text = v.Text
		case *chat.SystemMessage:
			text = v.Text
		default:
			continue
		}
		if text == "" {
			continue
		}
		if term, hit := m.matcher.Match(ctx, text); hit {
			return term, true
		}
	}
	return "", false
}

func blockError(scope SafeguardScope, term string) error {
	side := "input"
	if scope == ScopeOutput {
		side = "output"
	}
	if term == "" {
		return fmt.Errorf("%w (%s)", ErrUnsafeContent, side)
	}
	return fmt.Errorf("%w (%s: %q)", ErrUnsafeContent, side, term)
}

// passthroughCall is the no-op call middleware used when matcher is nil.
func passthroughCall(next chat.CallHandler) chat.CallHandler { return next }

// passthroughStream is the no-op stream middleware used when matcher is nil.
func passthroughStream(next chat.StreamHandler) chat.StreamHandler { return next }

// SubstringMatcherOptions configures [NewSubstringMatcher]. All
// fields default to false (the zero value) so a bare
// `SubstringMatcherOptions{}` produces the safe, verbose default.
type SubstringMatcherOptions struct {
	// CaseSensitive controls whether matching honors letter case.
	// Default false (terms compared after [strings.ToLower]).
	CaseSensitive bool

	// HideMatch suppresses the matched term in the resulting
	// [ErrUnsafeContent]. Default false: the matched term is
	// disclosed (useful for debugging). Set true when the term list
	// is sensitive (e.g. internal keyword detector you don't want
	// echoed back to callers).
	HideMatch bool
}

// NewSubstringMatcher is the stdlib-backed default [Matcher] —
// returns a hit when any term is contained in the scanned text.
// Empty / whitespace terms are dropped at construction time so a
// caller's "" sentinel never matches the universe.
//
// Performance: O(N×len(text)) per scan, no allocations beyond the
// terms slice itself. For large term lists or hot paths, plug in a
// dedicated multi-string matcher (Aho-Corasick, regex set) via the
// [Matcher] interface — substring is the safe-by-default minimum.
func NewSubstringMatcher(terms []string, opts ...SubstringMatcherOptions) Matcher {
	var opt SubstringMatcherOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cleaned := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !opt.CaseSensitive {
			t = strings.ToLower(t)
		}
		cleaned = append(cleaned, t)
	}
	return &substringMatcher{terms: cleaned, opts: opt}
}

type substringMatcher struct {
	terms []string
	opts  SubstringMatcherOptions
}

func (m *substringMatcher) Match(_ context.Context, text string) (string, bool) {
	if len(m.terms) == 0 || text == "" {
		return "", false
	}
	hay := text
	if !m.opts.CaseSensitive {
		hay = strings.ToLower(hay)
	}
	for _, t := range m.terms {
		if strings.Contains(hay, t) {
			if m.opts.HideMatch {
				return "", true
			}
			return t, true
		}
	}
	return "", false
}
