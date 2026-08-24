package operation

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestContractIsTheOnlyMethodTable(t *testing.T) {
	t.Parallel()

	metas := Contract().Metas()
	if len(metas) == 0 {
		t.Fatal("the contract registered no methods")
	}
	seen := make(map[string]bool, len(metas))
	for _, meta := range metas {
		if seen[meta.Name] {
			t.Errorf("method %q is registered twice", meta.Name)
		}
		seen[meta.Name] = true
		if err := meta.Validate(); err != nil {
			t.Errorf("%s: %v", meta.Name, err)
		}
	}
}

func TestStreamMethodsAreTheStreamingContract(t *testing.T) {
	t.Parallel()

	want := []string{"runs.start", "runs.resume", "runs.subscribe", "runtime.subscribe"}
	got := Contract().StreamMethods()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("stream methods = %v, want %v", got, want)
	}
}

func TestCapabilityRulesNameAPublishedFeature(t *testing.T) {
	t.Parallel()

	for _, meta := range Contract().Metas() {
		for _, feature := range meta.Features() {
			if _, published := protocol.LookupFeature(feature); !published {
				t.Errorf("%s requires %q, which protocol does not publish", meta.Name, feature)
			}
		}
	}
}

func TestMaterializedQueriesNameIndependentQueryFacts(t *testing.T) {
	t.Parallel()

	registry := Contract()
	for _, meta := range registry.Metas() {
		for _, name := range meta.Materializes {
			materialized, found := registry.Lookup(name)
			if !found {
				t.Errorf("%s materializes unregistered query %q", meta.Name, name)
				continue
			}
			if materialized.Operation != OperationQuery {
				t.Errorf("%s materializes %s, which is a %s", meta.Name, name, materialized.Operation)
			}
		}
	}
}

func TestReplayPolicyCoversEveryCommand(t *testing.T) {
	t.Parallel()

	for _, meta := range Contract().Metas() {
		switch meta.Operation {
		case OperationCommand:
			if !meta.Idempotency.Replays() {
				t.Errorf("%s: command has no replay protection", meta.Name)
			}
		case OperationQuery, OperationSubscription:
			if meta.Idempotency.Replays() {
				t.Errorf("%s: non-command unexpectedly keeps replay state", meta.Name)
			}
		}
	}
	for _, name := range []string{"runs.start", "runs.resume"} {
		method, _ := Contract().Lookup(name)
		if method.Idempotency != IdempotencyReplayRunStream {
			t.Errorf("%s must replay by re-attaching to its run, got %v", name, method.Idempotency)
		}
	}
}

func TestRunReplayCursorPolicyNamesOnlyReplayableRunStreams(t *testing.T) {
	t.Parallel()

	var got []string
	for _, meta := range Contract().Metas() {
		if meta.ReplayCursor == ReplayCursorRun {
			got = append(got, meta.Name)
		}
	}
	want := []string{"runs.resume", "runs.start", "runs.subscribe"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("run replay cursor methods = %v, want %v", got, want)
	}
}

func TestGoalManagementCommandsArePublished(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"goals.update", "goals.clear"} {
		method, ok := Contract().Lookup(name)
		if !ok {
			t.Errorf("%s is not registered", name)
			continue
		}
		if method.Operation != OperationCommand {
			t.Errorf("%s operation = %s, want %s", name, method.Operation, OperationCommand)
		}
		if !slices.Equal(method.Features(), []string{protocol.FeatureGoals}) {
			t.Errorf("%s features = %v, want [%s]", name, method.Features(), protocol.FeatureGoals)
		}
	}
}
