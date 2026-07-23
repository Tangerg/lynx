package runsegment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// Finish establishes the terminal file boundary before returning, then starts
// title generation off the live path. The checkpoint is a sequencing fence: the
// run admission remains held by the caller until it completes, so a following
// run cannot write into the preceding run's snapshot. Title generation does not
// define the boundary and may continue asynchronously. A parked run is
// resumable, not a boundary, so it does neither.
func (e *Effects) Finish(ctx context.Context, fin runs.Finish) error {
	if fin.Parked {
		return nil
	}
	needsSnapshot := e.checkpoints != nil && fin.Cwd != ""
	needsTitle := strings.TrimSpace(fin.OpeningUserText) != ""
	if !needsSnapshot && !needsTitle {
		return nil
	}
	var errs []error
	if needsSnapshot {
		if err := observeTerminalMaintenance(ctx, fin, "checkpoint", func(ctx context.Context) error {
			return e.snapshot(ctx, fin.SessionID, fin.Cwd, fin.RunID)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	if !needsTitle {
		return errors.Join(errs...)
	}
	title := func(ctx context.Context) error {
		return observeTerminalMaintenance(ctx, fin, "title", func(ctx context.Context) error {
			return e.title(ctx, fin.SessionID, fin.OpeningUserText)
		})
	}
	if e.tasks == nil {
		return errors.Join(append(errs, title(ctx))...)
	}
	if !e.tasks.Start(ctx, func(ctx context.Context) { _ = title(ctx) }) {
		rejected := fmt.Errorf("runsegment: terminal maintenance for run %q was rejected during shutdown", fin.RunID)
		errs = append(errs, observeTerminalMaintenance(ctx, fin, "title", func(context.Context) error { return rejected }))
	}
	return errors.Join(errs...)
}

func observeTerminalMaintenance(ctx context.Context, fin runs.Finish, operation string, maintenance func(context.Context) error) error {
	ctx, span := otel.Tracer(runsegmentTracerName).Start(ctx, "run terminal maintenance",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("run.id", fin.RunID),
			attribute.String("gen_ai.conversation.id", fin.SessionID),
			attribute.String("maintenance.operation", operation),
		),
	)
	defer span.End()
	err := maintenance(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (e *Effects) snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := e.checkpoints.Snapshot(ctx, sessionID, cwd, runID); err != nil {
		return fmt.Errorf("runsegment: snapshot workspace for run %q: %w", runID, err)
	}
	return nil
}

func (e *Effects) title(ctx context.Context, sessionID, prompt string) error {
	if e.sessions == nil {
		return errors.New("runsegment: session persistence is unavailable for title generation")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	sess, err := e.sessions.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runsegment: load session %q for title generation: %w", sessionID, err)
	}
	if strings.TrimSpace(sess.Title) != "" {
		return nil
	}
	if e.titles == nil {
		return errors.New("runsegment: title generation is unavailable")
	}
	title, err := e.titles.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("runsegment: generate title for session %q: %w", sessionID, err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("runsegment: generated title for session %q is empty", sessionID)
	}
	if err := e.sessions.RenameIfUntitled(ctx, sessionID, title); err != nil {
		return fmt.Errorf("runsegment: rename untitled session %q: %w", sessionID, err)
	}
	return nil
}
