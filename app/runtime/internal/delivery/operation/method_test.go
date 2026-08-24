package operation

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestRegistryViewsAreSnapshots(t *testing.T) {
	t.Parallel()

	var local MethodMeta
	for _, meta := range Contract().Metas() {
		if meta.Name == "sessions.update" {
			local = meta
			break
		}
	}
	if local.Name == "" || len(local.Errors) == 0 || len(local.CapabilityRules) == 0 {
		t.Fatal("sessions.update is missing mutable-slice metadata")
	}
	local.Errors[0] = protocol.ErrRunNotFound.Error()
	local.CapabilityRules[0].When[0].Field = "corrupted"
	local.CapabilityRules[0].Requires[0] = protocol.FeatureKnowledge
	got, ok := Contract().Lookup("sessions.update")
	if !ok || slices.Equal(got.Errors, local.Errors) ||
		got.CapabilityRules[0].When[0].Field == "corrupted" ||
		slices.Equal(got.CapabilityRules[0].Requires, []string{protocol.FeatureKnowledge}) {
		t.Fatalf("catalog exposed mutable storage: %+v", got)
	}

	snapshot, ok := Contract().Lookup("sessions.snapshot")
	if !ok || len(snapshot.Materializes) == 0 {
		t.Fatal("sessions.snapshot is missing materialized-query metadata")
	}
	snapshot.Materializes[0] = "corrupted.query"
	fresh, _ := Contract().Lookup("sessions.snapshot")
	if fresh.Materializes[0] == "corrupted.query" {
		t.Fatalf("catalog exposed mutable Materializes storage: %+v", fresh.Materializes)
	}
}

func TestMetadataEnumsRejectUnknownValuesWithoutMasqueradingAsDefaults(t *testing.T) {
	t.Parallel()

	registered, ok := Contract().Lookup("runs.list")
	if !ok {
		t.Fatal("runs.list is not registered")
	}

	tests := []struct {
		name   string
		mutate func(*MethodMeta)
		want   []string
	}{
		{name: "method name", mutate: func(meta *MethodMeta) { meta.Name = "runs" }, want: []string{`"runs"`, "dot-separated non-empty segments"}},
		{name: "method kind", mutate: func(meta *MethodMeta) { meta.Kind = MethodKind(255) }, want: []string{"runs.list", "MethodKind(255)"}},
		{name: "operation kind", mutate: func(meta *MethodMeta) { meta.Operation = OperationKind(255) }, want: []string{"runs.list", "OperationKind(255)"}},
		{name: "idempotency policy", mutate: func(meta *MethodMeta) { meta.Idempotency = IdempotencyPolicy(255) }, want: []string{"runs.list", "IdempotencyPolicy(255)"}},
		{name: "replay cursor policy", mutate: func(meta *MethodMeta) { meta.ReplayCursor = ReplayCursorPolicy(255) }, want: []string{"runs.list", "ReplayCursorPolicy(255)"}},
		{name: "query run replay cursor", mutate: func(meta *MethodMeta) { meta.ReplayCursor = ReplayCursorRun }, want: []string{"runs.list", "only a streaming method"}},
		{name: "pagination kind", mutate: func(meta *MethodMeta) { meta.Pagination = PaginationKind(255) }, want: []string{"runs.list", "PaginationKind(255)"}},
		{name: "pagination disagrees with shapes", mutate: func(meta *MethodMeta) { meta.Pagination = PaginationNone }, want: []string{"runs.list", "shapes derive cursor"}},
		{name: "condition operator", mutate: func(meta *MethodMeta) { meta.CapabilityRules[0].When[0].Operator = ConditionOperator(255) }, want: []string{"runs.list", "includeDescendants", "ConditionOperator(255)"}},
		{name: "materializes itself", mutate: func(meta *MethodMeta) { meta.Materializes = []string{meta.Name} }, want: []string{"runs.list", "cannot materialize itself"}},
		{name: "repeated materialized query", mutate: func(meta *MethodMeta) { meta.Materializes = []string{"items.list", "items.list"} }, want: []string{"runs.list", "items.list", "declared twice"}},
		{name: "command materialization", mutate: func(meta *MethodMeta) {
			meta.Materializes = []string{"items.list"}
			meta.Operation = OperationCommand
		}, want: []string{"runs.list", "only a query"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta := registered
			test.mutate(&meta)
			err := meta.Validate()
			if err == nil {
				t.Fatal("validate accepted an unknown metadata enum")
			}
			for _, fragment := range test.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}

	if got := MethodKind(255).String(); got == KindUnary.String() || got == KindStream.String() {
		t.Fatalf("unknown method kind masquerades as %q", got)
	}
	if got := OperationKind(255).String(); got == OperationQuery.String() {
		t.Fatalf("unknown operation kind masquerades as %q", got)
	}
	if got := IdempotencyPolicy(255).String(); got == IdempotencyNone.String() {
		t.Fatalf("unknown idempotency policy masquerades as %q", got)
	}
	if got := ReplayCursorPolicy(255).String(); got == ReplayCursorNone.String() {
		t.Fatalf("unknown replay cursor policy masquerades as %q", got)
	}
	if got := PaginationKind(255).String(); got == PaginationNone.String() {
		t.Fatalf("unknown pagination kind masquerades as %q", got)
	}
	if got := ConditionOperator(255).String(); got == OperatorPresent.String() {
		t.Fatalf("unknown condition operator masquerades as %q", got)
	}
}

func TestRunReplayCursorRequiresRunEventFrames(t *testing.T) {
	t.Parallel()

	meta, ok := Contract().Lookup("runtime.subscribe")
	if !ok {
		t.Fatal("runtime.subscribe is not registered")
	}
	meta.ReplayCursor = ReplayCursorRun
	if err := meta.Validate(); err == nil || !strings.Contains(err.Error(), "requires RunEvent stream frames") {
		t.Fatalf("Validate error = %v, want RunEvent stream refusal", err)
	}
}

func TestPaginationIsDerivedFromWireShapes(t *testing.T) {
	t.Parallel()

	wantCursor := []string{"interrupts.list", "items.list", "runs.list", "schedules.list", "sessions.list", "workspace.files.list"}
	var gotCursor []string
	for _, method := range Contract().Metas() {
		if method.Pagination == PaginationCursor {
			gotCursor = append(gotCursor, method.Name)
		}
	}
	slices.Sort(gotCursor)
	if !slices.Equal(gotCursor, wantCursor) {
		t.Fatalf("cursor-paginated methods = %v, want %v", gotCursor, wantCursor)
	}

	for _, name := range []string{"models.list", "providers.list", "runs.get", "tools.list", "workspaces.list"} {
		method, ok := Contract().Lookup(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if method.Pagination != PaginationNone {
			t.Errorf("%s pagination = %s, want %s", name, method.Pagination, PaginationNone)
		}
	}
}
