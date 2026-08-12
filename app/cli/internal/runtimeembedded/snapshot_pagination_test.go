package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type snapshotBindingStub struct {
	runs         map[string]*protocol.Page[protocol.RunRef]
	items        map[string]*protocol.ListItemsResponse
	interrupts   map[string]*protocol.Page[protocol.PendingInterruptSet]
	runRequests  *[]protocol.ListRunsRequest
	plan         *protocol.StateSnapshot
	planRequests *int
}

func (snapshotBindingStub) GetRun(context.Context, protocol.GetRunRequest, embedded.CallOptions) (*protocol.RunRef, error) {
	return nil, errors.New("unexpected GetRun")
}

func (snapshotBindingStub) GetSession(context.Context, protocol.GetSessionRequest, embedded.CallOptions) (*protocol.Session, error) {
	return nil, errors.New("unexpected GetSession")
}

func (stub snapshotBindingStub) GetPlan(context.Context, protocol.GetPlanRequest, embedded.CallOptions) (*protocol.StateSnapshot, error) {
	if stub.planRequests == nil {
		return nil, errors.New("unexpected GetPlan")
	}
	*stub.planRequests++
	return stub.plan, nil
}

func (stub snapshotBindingStub) ListRuns(_ context.Context, request protocol.ListRunsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.RunRef], error) {
	if stub.runRequests != nil {
		*stub.runRequests = append(*stub.runRequests, request)
	}
	return stub.runs[request.Cursor], nil
}

func (stub snapshotBindingStub) ListItems(_ context.Context, request protocol.ListItemsRequest, _ embedded.CallOptions) (*protocol.ListItemsResponse, error) {
	return stub.items[request.Cursor], nil
}

func TestSnapshotRunCatalogFollowsTheNegotiatedRunTopology(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("subagents=%t", enabled), func(t *testing.T) {
			t.Parallel()
			var requests []protocol.ListRunsRequest
			stub := snapshotBindingStub{
				runs:        map[string]*protocol.Page[protocol.RunRef]{"": protocol.NewPage([]protocol.RunRef{})},
				runRequests: &requests,
			}
			runtime := &Runtime{
				snapshot: stub, runCatalog: stub, meta: requestMeta("test"),
				profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
					runtimeprofile.FeatureSubagents: {
						Enabled: enabled, Stability: runtimeprofile.Stable,
						ClientOptIn: true, ClientRequested: enabled,
					},
				}},
			}
			if _, err := runtime.listAllRuns(t.Context(), "ses_1"); err != nil {
				t.Fatal(err)
			}
			if len(requests) != 1 || requests[0].SessionID != "ses_1" || requests[0].IncludeDescendants != enabled {
				t.Fatalf("list run requests = %+v, want descendants=%t", requests, enabled)
			}
		})
	}
}

func TestPlanColdReadFollowsThePublishedFeature(t *testing.T) {
	t.Parallel()
	requests := 0
	stub := snapshotBindingStub{
		plan:         &protocol.StateSnapshot{Type: protocol.StatePlan, SessionID: "ses_1"},
		planRequests: &requests,
	}
	runtime := &Runtime{snapshot: stub, meta: requestMeta("test")}
	if plan, err := runtime.readPlan(t.Context(), "ses_1"); err != nil || plan != nil || requests != 0 {
		t.Fatalf("disabled plan read = (%+v, %v), requests=%d", plan, err, requests)
	}
	runtime.profile.Features = map[runtimeprofile.FeatureName]runtimeprofile.Feature{
		runtimeprofile.FeaturePlan: {Enabled: true, Stability: runtimeprofile.Stable},
	}
	plan, err := runtime.readPlan(t.Context(), "ses_1")
	if err != nil || plan != stub.plan || requests != 1 {
		t.Fatalf("enabled plan read = (%+v, %v), requests=%d", plan, err, requests)
	}
}

func TestEnabledPlanColdReadRejectsMissingProjection(t *testing.T) {
	t.Parallel()
	requests := 0
	runtime := &Runtime{
		snapshot: snapshotBindingStub{planRequests: &requests},
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeaturePlan: {Enabled: true, Stability: runtimeprofile.Stable},
		}},
		meta: requestMeta("test"),
	}
	if plan, err := runtime.readPlan(t.Context(), "ses_1"); err == nil || plan != nil || requests != 1 {
		t.Fatalf("missing enabled plan read = (%+v, %v), requests=%d", plan, err, requests)
	}
}

func (stub snapshotBindingStub) ListInterrupts(_ context.Context, request protocol.ListInterruptsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return stub.interrupts[request.Cursor], nil
}

func TestSnapshotResourcesRejectMultiStepCursorCycles(t *testing.T) {
	t.Parallel()
	itemPage := func(next string) *protocol.ListItemsResponse {
		return &protocol.ListItemsResponse{Page: *protocol.NewPageWithCursor([]protocol.Item{}, next)}
	}
	tests := []struct {
		name string
		stub snapshotBindingStub
		call func(context.Context, *Runtime) error
	}{
		{
			name: "runs",
			stub: snapshotBindingStub{runs: map[string]*protocol.Page[protocol.RunRef]{
				"":       protocol.NewPageWithCursor([]protocol.RunRef{}, "first"),
				"first":  protocol.NewPageWithCursor([]protocol.RunRef{}, "second"),
				"second": protocol.NewPageWithCursor([]protocol.RunRef{}, "first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllRuns(ctx, "session")
				return err
			},
		},
		{
			name: "items",
			stub: snapshotBindingStub{items: map[string]*protocol.ListItemsResponse{
				"": itemPage("first"), "first": itemPage("second"), "second": itemPage("first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllItems(ctx, "session")
				return err
			},
		},
		{
			name: "interrupts",
			stub: snapshotBindingStub{interrupts: map[string]*protocol.Page[protocol.PendingInterruptSet]{
				"":       protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "first"),
				"first":  protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "second"),
				"second": protocol.NewPageWithCursor([]protocol.PendingInterruptSet{}, "first"),
			}},
			call: func(ctx context.Context, runtime *Runtime) error {
				_, err := runtime.listAllInterrupts(ctx, "session")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{snapshot: test.stub, runCatalog: test.stub, meta: requestMeta("test")}
			err := test.call(t.Context(), runtime)
			if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
				t.Fatalf("snapshot list error = %v, want cursor cycle failure", err)
			}
		})
	}
}

func TestColdReadStabilityAllowsMeteringProgressOnly(t *testing.T) {
	t.Parallel()
	base := coldRead{runs: []protocol.RunRef{{
		RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Status: protocol.RunStatusRunning,
		},
		ActiveSegmentID: "seg_1",
		Limits:          &protocol.RunLimits{MaxSteps: 12},
		ProtocolProfile: protocol.RunProtocolProfile{
			RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
			InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval},
		},
	}}}

	progressed := base
	progressed.runs = append([]protocol.RunRef(nil), base.runs...)
	progressed.runs[0].Metrics = protocol.RunMetrics{Steps: 3, ActiveDurationMillis: 250}
	if !coldReadsAgree(base, progressed) {
		t.Fatal("metering progress made an otherwise stable cold read disagree")
	}

	tests := []struct {
		name   string
		mutate func(*protocol.RunRef)
	}{
		{name: "lifecycle", mutate: func(run *protocol.RunRef) { run.Status = protocol.RunStatusWaiting }},
		{name: "active segment", mutate: func(run *protocol.RunRef) { run.ActiveSegmentID = "seg_2" }},
		{name: "limits", mutate: func(run *protocol.RunRef) { run.Limits = &protocol.RunLimits{MaxSteps: 24} }},
		{name: "protocol profile", mutate: func(run *protocol.RunRef) {
			run.ProtocolProfile.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			changed.runs = append([]protocol.RunRef(nil), base.runs...)
			test.mutate(&changed.runs[0])
			if coldReadsAgree(base, changed) {
				t.Fatal("lifecycle change was accepted as a stable cold read")
			}
		})
	}
}
