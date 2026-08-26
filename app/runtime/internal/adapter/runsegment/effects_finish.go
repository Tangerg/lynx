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

// SessionTitles is the Application capability for best-effort initial title
// generation. It rechecks first-writer semantics through Session behavior.
type SessionTitles interface {
	NeedsGeneratedTitle(ctx context.Context, id string) (bool, error)
	ApplyGeneratedTitle(ctx context.Context, id, title string) error
}

// TitleGenerator derives an initial session title from its opening request. It
// may return a usable deterministic fallback with a non-nil error so maintenance
// can persist stable identity and still report provider degradation.
type TitleGenerator interface {
	Generate(ctx context.Context, firstMessage string) (string, error)
}

// Checkpoints anchors the working tree at a terminal run boundary.
type Checkpoints interface {
	Snapshot(ctx context.Context, sessionID, cwd, runID string) error
}

// TaskLauncher starts request-detached work owned by its component lifecycle.
type TaskLauncher interface {
	Start(parent context.Context, task func(context.Context)) bool
}

// TitleMaintenance is the complete optional generated-title capability.
type TitleMaintenance struct {
	Sessions  SessionTitles
	Generator TitleGenerator
	Tasks     TaskLauncher
}

// FinalizerConfig declares the two independent terminal-maintenance features.
// A nil Checkpoints disables workspace snapshots; a nil Titles disables title
// generation. Enabled features must be complete at construction.
type FinalizerConfig struct {
	Checkpoints Checkpoints
	Titles      *TitleMaintenance
}

// Finalizer owns post-boundary maintenance. It is deliberately separate from
// durable Effects because neither title generation nor workspace snapshots
// participate in the Run transaction.
type Finalizer struct {
	checkpoints   Checkpoints
	sessionTitles SessionTitles
	titles        TitleGenerator
	tasks         TaskLauncher
}

var _ runs.SegmentFinalizer = (*Finalizer)(nil)

func NewFinalizer(cfg FinalizerConfig) (*Finalizer, error) {
	if cfg.Checkpoints != nil && nilDependency(cfg.Checkpoints) {
		return nil, errors.New("runsegment: optional checkpoints must not be typed nil")
	}
	finalizer := &Finalizer{checkpoints: cfg.Checkpoints}
	if cfg.Titles == nil {
		return finalizer, nil
	}
	if nilDependency(cfg.Titles.Sessions) {
		return nil, errors.New("runsegment: session titles are required when title maintenance is enabled")
	}
	if nilDependency(cfg.Titles.Generator) {
		return nil, errors.New("runsegment: title generator is required when title maintenance is enabled")
	}
	if nilDependency(cfg.Titles.Tasks) {
		return nil, errors.New("runsegment: task launcher is required when title maintenance is enabled")
	}
	finalizer.sessionTitles = cfg.Titles.Sessions
	finalizer.titles = cfg.Titles.Generator
	finalizer.tasks = cfg.Titles.Tasks
	return finalizer, nil
}

// Finish establishes the terminal file boundary before returning, then starts
// title generation off the live path. The checkpoint is a sequencing fence: the
// run admission remains held by the caller until it completes, so a following
// run cannot write into the preceding run's snapshot. Title generation does not
// define the boundary and may continue asynchronously. A parked run is
// resumable, not a boundary, so it does neither.
func (f *Finalizer) Finish(ctx context.Context, fin runs.Finish) error {
	if fin.Parked {
		return nil
	}
	needsSnapshot := f.checkpoints != nil && fin.CWD != ""
	needsTitle := f.sessionTitles != nil && strings.TrimSpace(fin.OpeningUserText) != ""
	if !needsSnapshot && !needsTitle {
		return nil
	}
	var errs []error
	if needsSnapshot {
		if err := observeTerminalMaintenance(ctx, fin, "checkpoint", func(ctx context.Context) error {
			return f.snapshot(ctx, fin.SessionID, fin.CWD, fin.RunID)
		}); err != nil {
			errs = append(errs, err)
		}
	}
	if !needsTitle {
		return errors.Join(errs...)
	}
	title := func(ctx context.Context) error {
		return observeTerminalMaintenance(ctx, fin, "title", func(ctx context.Context) error {
			return f.title(ctx, fin.SessionID, fin.OpeningUserText)
		})
	}
	if !f.tasks.Start(ctx, func(ctx context.Context) { _ = title(ctx) }) {
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

func (f *Finalizer) snapshot(ctx context.Context, sessionID, cwd, runID string) error {
	if err := f.checkpoints.Snapshot(ctx, sessionID, cwd, runID); err != nil {
		return fmt.Errorf("runsegment: snapshot workspace for run %q: %w", runID, err)
	}
	return nil
}

func (f *Finalizer) title(ctx context.Context, sessionID, prompt string) error {
	if f.sessionTitles == nil {
		return errors.New("runsegment: Session title use cases are unavailable")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	needed, err := f.sessionTitles.NeedsGeneratedTitle(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("runsegment: inspect Session %q for title generation: %w", sessionID, err)
	}
	if !needed {
		return nil
	}
	if f.titles == nil {
		return errors.New("runsegment: title generation is unavailable")
	}
	title, generationErr := f.titles.Generate(ctx, prompt)
	title = strings.TrimSpace(title)
	if title == "" && generationErr == nil {
		return fmt.Errorf("runsegment: generated title for session %q is empty", sessionID)
	}
	if title != "" {
		if err := f.sessionTitles.ApplyGeneratedTitle(ctx, sessionID, title); err != nil {
			applyErr := fmt.Errorf("runsegment: apply generated title to Session %q: %w", sessionID, err)
			if generationErr != nil {
				return errors.Join(
					fmt.Errorf("runsegment: generate title for session %q: %w", sessionID, generationErr),
					applyErr,
				)
			}
			return applyErr
		}
	}
	if generationErr != nil {
		return fmt.Errorf("runsegment: generate title for session %q: %w", sessionID, generationErr)
	}
	return nil
}
