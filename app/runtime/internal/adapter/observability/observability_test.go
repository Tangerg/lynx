package observability

import (
	"context"
	"io"
	stdslog "log/slog"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
)

// TestSetupObservability_LogPathEmits drives the REAL setup path
// end to end with os.Stderr redirected, then emits a business log via the
// package-level slog.InfoContext under an active span — the exact runtime
// shape. It guards the whole triad-to-slog log path (slog → minLevelHandler
// → otelslog bridge → LoggerProvider → LogExporter → stderr) against a
// silent drop.
func TestSetupObservability_LogPathEmits(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w // Setup captures os.Stderr at call time

	prevDefault := stdslog.Default()
	shutdown := Setup("test")

	ctx, span := otel.Tracer("test").Start(context.Background(), "req")
	stdslog.InfoContext(ctx, "session created", "gen_ai.conversation.id", "ses_x")
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	_ = w.Close()
	os.Stderr = origStderr
	stdslog.SetDefault(prevDefault)
	t.Cleanup(func() { otel.SetTracerProvider(nil); otel.SetMeterProvider(nil) })

	out, _ := io.ReadAll(r)
	got := string(out)
	if !strings.Contains(got, "session created") {
		t.Fatalf("business log dropped by the real setup path. output:\n%s", got)
	}
	if !strings.Contains(got, "ses_x") {
		t.Fatalf("log attribute missing. output:\n%s", got)
	}
}
