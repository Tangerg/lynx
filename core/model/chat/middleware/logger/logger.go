package logger

import (
	"context"
	"iter"
	"log/slog"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Logger is the dependency-inverted sink that [NewMiddleware]
// delegates to. Implementations decide *how* to record what the
// middleware observes — slog, zap, an OTel log bridge, a metrics
// counter, or a no-op for benchmarks. Lynx ships [NewSlogLogger] as
// the default.
//
// Methods receive the original request and (where applicable) the
// response, error and wall-clock latency for that handler invocation.
// Implementations must be safe for concurrent use: a single Logger is
// shared by every in-flight chat call.
type Logger interface {
	// LogRequest is called once per chat invocation, before the
	// underlying handler runs.
	LogRequest(ctx context.Context, req *chat.Request)

	// LogResponse is called once the handler returns successfully.
	// For streams, resp is the fully-accumulated response.
	LogResponse(ctx context.Context, req *chat.Request, resp *chat.Response, latency time.Duration)

	// LogError is called when the handler returns a non-nil error.
	// resp is whatever partial value the handler produced (often nil).
	LogError(ctx context.Context, req *chat.Request, err error, latency time.Duration)
}

// NewMiddleware returns a (call, stream) middleware pair that
// emits one request event before the handler runs and one
// response / error event after. The pair shares a single [Logger]
// instance, so the middleware adds zero allocations beyond the
// observability work the Logger itself performs.
//
// Example:
//
//	callMW, streamMW := middleware.NewMiddleware(
//	    middleware.NewSlogLogger(slog.Default()),
//	)
//	resp, err := client.Chat().
//	    WithMiddlewares(callMW, streamMW).
//	    WithUserPrompt("hi").
//	    Call().Response(ctx)
//
// Passing nil falls back to a no-op Logger so callers never need to
// nil-check.
func NewMiddleware(logger Logger) (chat.CallMiddleware, chat.StreamMiddleware) {
	if logger == nil {
		logger = nopLogger{}
	}
	mw := &loggerMiddleware{logger: logger}
	return mw.wrapCall, mw.wrapStream
}

type loggerMiddleware struct {
	logger Logger
}

func (m *loggerMiddleware) wrapCall(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		m.logger.LogRequest(ctx, req)
		started := time.Now()
		resp, err := next.Call(ctx, req)
		latency := time.Since(started)
		if err != nil {
			m.logger.LogError(ctx, req, err, latency)
			return resp, err
		}
		m.logger.LogResponse(ctx, req, resp, latency)
		return resp, nil
	})
}

func (m *loggerMiddleware) wrapStream(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			m.logger.LogRequest(ctx, req)
			started := time.Now()
			acc := chat.NewResponseAccumulator()
			var streamErr error

			for resp, err := range next.Stream(ctx, req) {
				if err != nil {
					streamErr = err
					if !yield(resp, err) {
						return
					}
					continue
				}
				acc.AddChunk(resp)
				if !yield(resp, nil) {
					return
				}
			}

			latency := time.Since(started)
			if streamErr != nil {
				m.logger.LogError(ctx, req, streamErr, latency)
				return
			}
			m.logger.LogResponse(ctx, req, &acc.Response, latency)
		}
	})
}

// nopLogger is the safe fallback used when callers pass nil.
type nopLogger struct{}

func (nopLogger) LogRequest(context.Context, *chat.Request)                                 {}
func (nopLogger) LogResponse(context.Context, *chat.Request, *chat.Response, time.Duration) {}
func (nopLogger) LogError(context.Context, *chat.Request, error, time.Duration)             {}

// SlogLoggerOption configures [NewSlogLogger].
type SlogLoggerOption func(*slogLogger)

// WithSlogRequestLevel overrides the slog level used for request
// events (default [slog.LevelInfo]).
func WithSlogRequestLevel(level slog.Level) SlogLoggerOption {
	return func(l *slogLogger) { l.requestLevel = level }
}

// WithSlogResponseLevel overrides the slog level used for response
// events (default [slog.LevelInfo]).
func WithSlogResponseLevel(level slog.Level) SlogLoggerOption {
	return func(l *slogLogger) { l.responseLevel = level }
}

// WithSlogErrorLevel overrides the slog level used for error events
// (default [slog.LevelError]).
func WithSlogErrorLevel(level slog.Level) SlogLoggerOption {
	return func(l *slogLogger) { l.errorLevel = level }
}

// NewSlogLogger is the stdlib-backed default [Logger] — emits one
// structured slog record per request / response / error. When logger
// is nil, falls back to [slog.Default]().
//
// Attribute schema (stable):
//
//	gen_ai.system           — provider id (model.Metadata().Provider, lowercased)
//	gen_ai.request.model    — request.Options.Model
//	gen_ai.message.count    — len(request.Messages)
//	gen_ai.duration_ms      — wall-clock latency in ms (response/error)
//	gen_ai.finish_reason    — response.Result.Metadata.FinishReason
//	gen_ai.usage.*_tokens   — usage counters (response only, when present)
//	error.message           — error.Error() (error events)
//
// The schema matches `OBSERVABILITY.md §3` so spans + logs share keys.
func NewSlogLogger(logger *slog.Logger, opts ...SlogLoggerOption) Logger {
	if logger == nil {
		logger = slog.Default()
	}
	l := &slogLogger{
		logger:        logger,
		requestLevel:  slog.LevelInfo,
		responseLevel: slog.LevelInfo,
		errorLevel:    slog.LevelError,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

type slogLogger struct {
	logger        *slog.Logger
	requestLevel  slog.Level
	responseLevel slog.Level
	errorLevel    slog.Level
}

func (l *slogLogger) LogRequest(ctx context.Context, req *chat.Request) {
	l.logger.LogAttrs(ctx, l.requestLevel, "chat.request", baseAttrs(req)...)
}

func (l *slogLogger) LogResponse(ctx context.Context, req *chat.Request, resp *chat.Response, latency time.Duration) {
	attrs := baseAttrs(req)
	attrs = append(attrs, slog.Int64("gen_ai.duration_ms", latency.Milliseconds()))
	if resp != nil && resp.Result != nil && resp.Result.Metadata != nil {
		if fr := resp.Result.Metadata.FinishReason; fr != "" {
			attrs = append(attrs, slog.String("gen_ai.finish_reason", string(fr)))
		}
	}
	if resp != nil && resp.Metadata != nil && resp.Metadata.Usage != nil {
		u := resp.Metadata.Usage
		if u.PromptTokens > 0 {
			attrs = append(attrs, slog.Int64("gen_ai.usage.input_tokens", u.PromptTokens))
		}
		if u.CompletionTokens > 0 {
			attrs = append(attrs, slog.Int64("gen_ai.usage.output_tokens", u.CompletionTokens))
		}
		if total := u.TotalTokens(); total > 0 {
			attrs = append(attrs, slog.Int64("gen_ai.usage.total_tokens", total))
		}
	}
	l.logger.LogAttrs(ctx, l.responseLevel, "chat.response", attrs...)
}

func (l *slogLogger) LogError(ctx context.Context, req *chat.Request, err error, latency time.Duration) {
	attrs := baseAttrs(req)
	attrs = append(attrs,
		slog.Int64("gen_ai.duration_ms", latency.Milliseconds()),
		slog.String("error.message", err.Error()),
	)
	l.logger.LogAttrs(ctx, l.errorLevel, "chat.error", attrs...)
}

func baseAttrs(req *chat.Request) []slog.Attr {
	if req == nil {
		return nil
	}
	attrs := []slog.Attr{
		slog.Int("gen_ai.message.count", len(req.Messages)),
	}
	if req.Options != nil && req.Options.Model != "" {
		attrs = append(attrs, slog.String("gen_ai.request.model", req.Options.Model))
	}
	return attrs
}
