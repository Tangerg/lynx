package slog

import (
	"bytes"
	"context"
	stdslog "log/slog"
	"strings"
	"testing"

	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// TestLogExporter_EmitViaProvider drives the real path a production app
// uses — emit through a LoggerProvider whose processor feeds the exporter —
// and asserts the record lands in the slog stream with body + attrs. It also
// pins down the SDK's Enabled semantics (a SimpleProcessor-only provider
// must report enabled, else the otelslog bridge would silently drop logs).
func TestLogExporter_EmitViaProvider(t *testing.T) {
	var buf bytes.Buffer
	logger := stdslog.New(stdslog.NewTextHandler(&buf, &stdslog.HandlerOptions{Level: stdslog.LevelInfo}))

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(NewLogExporter(logger))),
	)
	t.Cleanup(func() { _ = lp.Shutdown(context.Background()) })

	l := lp.Logger("test/scope")

	if !l.Enabled(context.Background(), otellog.EnabledParameters{Severity: otellog.SeverityInfo}) {
		t.Fatal("Logger.Enabled returned false for a SimpleProcessor provider — the otelslog bridge would drop every record")
	}

	var rec otellog.Record
	rec.SetSeverity(otellog.SeverityInfo)
	rec.SetBody(otellog.StringValue("session created"))
	rec.AddAttributes(otellog.String("gen_ai.conversation.id", "ses_42"))
	l.Emit(context.Background(), rec)

	out := buf.String()
	if !strings.Contains(out, "session created") {
		t.Fatalf("body not in output: %q", out)
	}
	if !strings.Contains(out, "ses_42") {
		t.Fatalf("attribute not in output: %q", out)
	}
}
