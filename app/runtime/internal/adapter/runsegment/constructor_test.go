package runsegment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestNewRejectsMalformedDependencies(t *testing.T) {
	var typedNilInterrupts *fakeInterrupts
	for _, test := range []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "empty", cfg: Config{}, want: "interrupt store"},
		{name: "typed nil", cfg: Config{Interrupts: typedNilInterrupts}, want: "interrupt store"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNewFinalizerRejectsPartialTitleMaintenance(t *testing.T) {
	_, err := NewFinalizer(FinalizerConfig{Titles: &TitleMaintenance{}})
	if err == nil || !strings.Contains(err.Error(), "session titles") {
		t.Fatalf("NewFinalizer error = %v", err)
	}
}

func mustNewEffects(cfg Config) *Effects {
	if nilDependency(cfg.ScheduleFirings) {
		cfg.ScheduleFirings = nil
	}
	if nilDependency(cfg.GoalRuns) {
		cfg.GoalRuns = nil
	}
	if nilDependency(cfg.ToolResults) {
		cfg.ToolResults = nil
	}
	interrupts := &fakeInterrupts{}
	if nilDependency(cfg.Interrupts) {
		cfg.Interrupts = interrupts
	}
	if nilDependency(cfg.ResumeClaims) {
		if claims, ok := cfg.Interrupts.(ResumeClaimStore); ok {
			cfg.ResumeClaims = claims
		} else {
			cfg.ResumeClaims = interrupts
		}
	}
	if nilDependency(cfg.Sessions) {
		cfg.Sessions = &fakeSession{}
	}
	if nilDependency(cfg.Transcript) {
		cfg.Transcript = &fakeTranscript{}
	}
	items := inertItems{}
	if nilDependency(cfg.ItemReplacer) {
		if replacer, ok := cfg.Transcript.(ItemReplacer); ok {
			cfg.ItemReplacer = replacer
		} else {
			cfg.ItemReplacer = items
		}
	}
	if nilDependency(cfg.ToolApprovals) {
		if approvals, ok := cfg.Transcript.(ToolApprovalStore); ok {
			cfg.ToolApprovals = approvals
		} else {
			cfg.ToolApprovals = items
		}
	}
	if nilDependency(cfg.ModelInvocations) {
		cfg.ModelInvocations = inertModelInvocations{}
	}
	if nilDependency(cfg.ToolInvocations) {
		cfg.ToolInvocations = inertToolInvocations{}
	}
	if nilDependency(cfg.Conversation) {
		cfg.Conversation = &fakeStores{}
	}
	if nilDependency(cfg.State) {
		cfg.State = &fakeRunState{}
	}
	if nilDependency(cfg.RunMetrics) {
		if metrics, ok := cfg.State.(RunMetricsWriter); ok {
			cfg.RunMetrics = metrics
		} else {
			cfg.RunMetrics = &fakeRunState{}
		}
	}
	if nilDependency(cfg.ExecutorCheckpoints) {
		cfg.ExecutorCheckpoints = &recordingExecutorCheckpointStore{}
	}
	if nilDependency(cfg.ChildRunStarts) {
		cfg.ChildRunStarts = &fakeChildRunStarts{}
	}
	if nilDependency(cfg.Tx) {
		cfg.Tx = func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }
	}
	effects, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return effects
}

func mustNewFinalizer(cfg FinalizerConfig) *Finalizer {
	finalizer, err := NewFinalizer(cfg)
	if err != nil {
		panic(err)
	}
	return finalizer
}

type inertItems struct{}

func (inertItems) Item(context.Context, string) (transcript.Item, bool, error) {
	return transcript.Item{}, false, nil
}

func (inertItems) ReplaceItem(context.Context, transcript.Item, transcript.Item) error {
	return nil
}

type inertModelInvocations struct{}

func (inertModelInvocations) StartModelInvocation(context.Context, string, string, string, string, time.Time) error {
	return nil
}
func (inertModelInvocations) CompleteModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertModelInvocations) FailModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertModelInvocations) MarkModelInvocationUnknown(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}

type inertToolInvocations struct{}

func (inertToolInvocations) StartToolInvocation(context.Context, string, string, string, string, string, time.Time) error {
	return nil
}
func (inertToolInvocations) CompleteToolInvocation(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertToolInvocations) MarkToolInvocationIncomplete(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}
