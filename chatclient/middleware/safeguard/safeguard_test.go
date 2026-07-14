package safeguard_test

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/chatclient/middleware/safeguard"
	"github.com/Tangerg/lynx/core/chat"
)

func TestScopeContract(t *testing.T) {
	for scope, name := range map[safeguard.Scope]string{
		safeguard.ScopeInput:  "input",
		safeguard.ScopeOutput: "output",
		safeguard.ScopeBoth:   "input+output",
	} {
		if !scope.Valid() || scope.String() != name {
			t.Fatalf("scope %d = valid %v, string %q", scope, scope.Valid(), scope.String())
		}
	}
	invalid := safeguard.Scope(8)
	if invalid.Valid() || invalid.String() != "Scope(8)" {
		t.Fatalf("invalid scope = valid %v, string %q", invalid.Valid(), invalid.String())
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	if _, err := safeguard.New(nil, safeguard.Config{}); !errors.Is(err, safeguard.ErrInvalidConfig) {
		t.Fatalf("nil matcher error = %v", err)
	}
	matcher := safeguard.MatcherFunc(func(context.Context, string) (safeguard.Match, error) {
		return safeguard.Match{}, nil
	})
	if _, err := safeguard.New(matcher, safeguard.Config{Scope: 8}); !errors.Is(err, safeguard.ErrInvalidConfig) {
		t.Fatalf("invalid scope error = %v", err)
	}
	if _, err := safeguard.NewSubstringMatcher([]string{"", "  "}, safeguard.SubstringOptions{}); !errors.Is(err, safeguard.ErrInvalidConfig) {
		t.Fatalf("empty terms error = %v", err)
	}
}

func TestSubstringMatcherSnapshotsTermsAndSupportsDisclosurePolicy(t *testing.T) {
	terms := []string{" SECRET ", "secret", "other"}
	matcher, err := safeguard.NewSubstringMatcher(terms, safeguard.SubstringOptions{})
	if err != nil {
		t.Fatal(err)
	}
	terms[0] = "mutated"
	match, err := matcher.Match(t.Context(), "contains secret")
	if err != nil || !match.Found || match.Term != "SECRET" {
		t.Fatalf("case-insensitive match = %#v, %v", match, err)
	}
	if match, err := matcher.Match(t.Context(), "clean"); err != nil || match.Found {
		t.Fatalf("clean match = %#v, %v", match, err)
	}

	sensitive, err := safeguard.NewSubstringMatcher([]string{"SECRET"}, safeguard.SubstringOptions{CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if match, _ := sensitive.Match(t.Context(), "secret"); match.Found {
		t.Fatal("case-sensitive matcher accepted lowercase text")
	}
	hidden, err := safeguard.NewSubstringMatcher([]string{"secret"}, safeguard.SubstringOptions{HideMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if match, _ := hidden.Match(t.Context(), "secret"); !match.Found || match.Term != "" {
		t.Fatalf("hidden match = %#v", match)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := matcher.Match(ctx, "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestCallBlocksInputBeforeModelAndReportsBlock(t *testing.T) {
	matcher := mustSubstring(t, "secret")
	var blocks []safeguard.Block
	middleware := mustMiddleware(t, matcher, safeguard.Config{
		Scope: safeguard.ScopeInput,
		OnBlock: func(_ context.Context, block safeguard.Block) {
			blocks = append(blocks, block)
		},
	})
	called := false
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		called = true
		return response("unused"), nil
	})
	request := mustRequest(t,
		chat.NewSystemMessage("rules"),
		chat.NewUserMessage(chat.NewTextPart("show the SECRET")),
	)
	got, err := middleware.Call(model).Call(t.Context(), request)
	if got != nil || !errors.Is(err, safeguard.ErrUnsafeContent) {
		t.Fatalf("Call result = %#v, %v", got, err)
	}
	var unsafe *safeguard.UnsafeError
	if !errors.As(err, &unsafe) || unsafe.Block.Scope != safeguard.ScopeInput || unsafe.Block.Term != "secret" {
		t.Fatalf("unsafe error = %#v", unsafe)
	}
	if called || !reflect.DeepEqual(blocks, []safeguard.Block{{Scope: safeguard.ScopeInput, Term: "secret"}}) {
		t.Fatalf("called=%v blocks=%#v", called, blocks)
	}
}

func TestCallScansEveryOutputChoiceBeforeDisclosure(t *testing.T) {
	middleware := mustMiddleware(t, mustSubstring(t, "blocked"), safeguard.Config{Scope: safeguard.ScopeOutput})
	modelResponse := response("safe", "second is blocked")
	got, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return modelResponse, nil
	})).Call(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
	if got != nil || !errors.Is(err, safeguard.ErrUnsafeContent) {
		t.Fatalf("output block result = %#v, %v", got, err)
	}
}

func TestCallHonorsScopeAndPreservesModelAndMatcherErrors(t *testing.T) {
	request := mustRequest(t, chat.NewUserMessage(chat.NewTextPart("secret input")))
	outputOnly := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{Scope: safeguard.ScopeOutput})
	if got, err := outputOnly.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response("clean"), nil
	})).Call(t.Context(), request); err != nil || got.Text() != "clean" {
		t.Fatalf("output-only result = %#v, %v", got, err)
	}

	modelErr := errors.New("model failed")
	modelResponse := response("partial")
	got, err := outputOnly.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return modelResponse, modelErr
	})).Call(t.Context(), request)
	if got != modelResponse || !errors.Is(err, modelErr) {
		t.Fatalf("model error result = %p, %v", got, err)
	}

	matchErr := errors.New("matcher failed")
	failing := safeguard.MatcherFunc(func(context.Context, string) (safeguard.Match, error) {
		return safeguard.Match{}, matchErr
	})
	middleware := mustMiddleware(t, failing, safeguard.Config{Scope: safeguard.ScopeOutput})
	got, err = middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response("answer"), nil
	})).Call(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
	if got != nil || !errors.Is(err, matchErr) {
		t.Fatalf("matcher error result = %#v, %v", got, err)
	}
}

func TestCallInputIgnoresPriorAssistantAndToolMessages(t *testing.T) {
	middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{Scope: safeguard.ScopeInput})
	request := mustRequest(t,
		chat.NewAssistantMessage(chat.NewTextPart("secret")),
		chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "lookup", Result: "secret"}),
		chat.NewUserMessage(chat.NewTextPart("clean")),
	)
	if got, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response("answer"), nil
	})).Call(t.Context(), request); err != nil || got.Text() != "answer" {
		t.Fatalf("Call result = %#v, %v", got, err)
	}
}

func TestStreamDetectsMatchesSplitAcrossChunksBeforeYieldingTrigger(t *testing.T) {
	middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{Scope: safeguard.ScopeOutput})
	closed := false
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			defer func() { closed = true }()
			if !yield(chunk("se"), nil) {
				return
			}
			yield(chunk("cret"), nil)
		}
	})
	var texts []string
	var gotErr error
	for response, err := range middleware.Stream(streamer).Stream(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
		if err != nil {
			gotErr = err
			continue
		}
		texts = append(texts, response.Text())
	}
	if !closed || !reflect.DeepEqual(texts, []string{"se"}) || !errors.Is(gotErr, safeguard.ErrUnsafeContent) {
		t.Fatalf("stream result closed=%v texts=%v error=%v", closed, texts, gotErr)
	}
}

func TestStreamRejectsUnsafeInputWithoutStartingProvider(t *testing.T) {
	middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{})
	started := false
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		started = true
		return func(func(*chat.Response, error) bool) {}
	})
	var gotErr error
	for _, err := range middleware.Stream(streamer).Stream(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("secret")))) {
		gotErr = err
	}
	if started || !errors.Is(gotErr, safeguard.ErrUnsafeContent) {
		t.Fatalf("stream started=%v error=%v", started, gotErr)
	}
}

func TestStreamPreservesEarlyStopAndProviderFailures(t *testing.T) {
	t.Run("early stop", func(t *testing.T) {
		middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{})
		closed := false
		streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				defer func() { closed = true }()
				if !yield(chunk("one"), nil) {
					return
				}
				yield(chunk("two"), nil)
			}
		})
		middleware.Stream(streamer).Stream(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))(func(*chat.Response, error) bool {
			return false
		})
		if !closed {
			t.Fatal("provider resources were not released")
		}
	})

	t.Run("provider error", func(t *testing.T) {
		providerErr := errors.New("provider failed")
		middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{})
		streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) { yield(nil, providerErr) }
		})
		var gotErr error
		for _, err := range middleware.Stream(streamer).Stream(t.Context(), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
			gotErr = err
		}
		if !errors.Is(gotErr, providerErr) {
			t.Fatalf("provider error = %v", gotErr)
		}
	})
}

func TestStreamReportsNilSequenceAndMalformedChunk(t *testing.T) {
	middleware := mustMiddleware(t, mustSubstring(t, "secret"), safeguard.Config{})
	request := mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))
	nilStreamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] { return nil })
	var gotErr error
	for _, err := range middleware.Stream(nilStreamer).Stream(t.Context(), request) {
		gotErr = err
	}
	if !errors.Is(gotErr, safeguard.ErrNilStream) {
		t.Fatalf("nil stream error = %v", gotErr)
	}

	badStreamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			yield(&chat.Response{Choices: []chat.Choice{{Index: -1}}}, nil)
		}
	})
	gotErr = nil
	for _, err := range middleware.Stream(badStreamer).Stream(t.Context(), request) {
		gotErr = err
	}
	if !errors.Is(gotErr, chat.ErrInvalidResponse) {
		t.Fatalf("malformed chunk error = %v", gotErr)
	}
}

func TestUnsafeErrorNilReceiver(t *testing.T) {
	var unsafe *safeguard.UnsafeError
	if unsafe.Error() != safeguard.ErrUnsafeContent.Error() || !errors.Is(unsafe, safeguard.ErrUnsafeContent) {
		t.Fatalf("nil unsafe error = %q", unsafe.Error())
	}
}

func mustSubstring(t *testing.T, terms ...string) *safeguard.SubstringMatcher {
	t.Helper()
	matcher, err := safeguard.NewSubstringMatcher(terms, safeguard.SubstringOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return matcher
}

func mustMiddleware(t *testing.T, matcher safeguard.Matcher, config safeguard.Config) *safeguard.Middleware {
	t.Helper()
	middleware, err := safeguard.New(matcher, config)
	if err != nil {
		t.Fatal(err)
	}
	return middleware
}

func mustRequest(t *testing.T, messages ...chat.Message) *chat.Request {
	t.Helper()
	request, err := chat.NewRequest(messages...)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func response(texts ...string) *chat.Response {
	choices := make([]chat.Choice, len(texts))
	for i, text := range texts {
		message := chat.NewAssistantMessage(chat.NewTextPart(text))
		choices[i] = chat.Choice{Index: i, Message: &message, FinishReason: chat.FinishReasonStop}
	}
	return &chat.Response{Choices: choices}
}

func chunk(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &message}}}
}
