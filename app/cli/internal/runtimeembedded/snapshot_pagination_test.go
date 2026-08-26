package runtimeembedded

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type snapshotBindingStub struct {
	sessions         []*protocol.Session
	sessionAt        func(int) *protocol.Session
	sessionCalls     int
	snapshot         *protocol.SessionSnapshot
	snapshotErr      error
	snapshotRequests []protocol.GetSessionSnapshotRequest
}

func (stub *snapshotBindingStub) GetSession(
	context.Context,
	protocol.GetSessionRequest,
	embedded.CallOptions,
) (*protocol.Session, error) {
	stub.sessionCalls++
	if stub.sessionAt != nil {
		return stub.sessionAt(stub.sessionCalls), nil
	}
	if len(stub.sessions) == 0 {
		return nil, nil
	}
	index := min(stub.sessionCalls-1, len(stub.sessions)-1)
	return stub.sessions[index], nil
}

func (stub *snapshotBindingStub) GetSessionSnapshot(
	_ context.Context,
	request protocol.GetSessionSnapshotRequest,
	_ embedded.CallOptions,
) (*protocol.SessionSnapshot, error) {
	stub.snapshotRequests = append(stub.snapshotRequests, request)
	return stub.snapshot, stub.snapshotErr
}

func snapshotSession(revision uint64) *protocol.Session {
	return &protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle, Revision: revision,
		Provider: testSessionProvider, Model: testSessionModel,
		Workspace: testProtocolWorkspace("/workspace", "/workspace", protocol.WorkspaceAvailable),
	}
}

func snapshotProfile(features ...runtimeprofile.FeatureName) runtimeprofile.Profile {
	profile := runtimeprofile.Profile{Features: make(map[runtimeprofile.FeatureName]runtimeprofile.Feature)}
	for _, feature := range features {
		profile.Features[feature] = runtimeprofile.Feature{
			Enabled: true, ClientOptIn: feature == runtimeprofile.FeatureSubagents,
			ClientRequested: feature == runtimeprofile.FeatureSubagents,
		}
	}
	return profile
}

func TestSessionMaterialSnapshotFollowsTheNegotiatedTopology(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "roots", true: "descendants"}[enabled], func(t *testing.T) {
			stub := &snapshotBindingStub{
				sessions: []*protocol.Session{snapshotSession(1)}, snapshot: &protocol.SessionSnapshot{},
			}
			profile := snapshotProfile()
			if enabled {
				profile = snapshotProfile(runtimeprofile.FeatureSubagents)
			}
			runtime := &Runtime{snapshot: stub, profile: profile, meta: requestMeta("test")}
			if _, err := runtime.GetSession(t.Context(), "ses_1"); err != nil {
				t.Fatal(err)
			}
			if len(stub.snapshotRequests) != 1 {
				t.Fatalf("snapshot requests = %d, want 1", len(stub.snapshotRequests))
			}
			request := stub.snapshotRequests[0]
			if request.SessionID != "ses_1" || request.IncludeDescendants != enabled {
				t.Fatalf("snapshot request = %+v, want descendants=%t", request, enabled)
			}
		})
	}
}

func TestSessionMaterialSnapshotEnforcesThePublishedPlanShape(t *testing.T) {
	tests := []struct {
		name    string
		profile runtimeprofile.Profile
		plan    *protocol.Plan
		wantErr bool
	}{
		{name: "disabled and absent", profile: snapshotProfile()},
		{name: "disabled but present", profile: snapshotProfile(), plan: &protocol.Plan{}, wantErr: true},
		{
			name: "enabled and present", profile: snapshotProfile(runtimeprofile.FeaturePlan),
			plan: &protocol.Plan{SessionID: "ses_1"},
		},
		{name: "enabled but absent", profile: snapshotProfile(runtimeprofile.FeaturePlan), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &snapshotBindingStub{snapshot: &protocol.SessionSnapshot{Plan: test.plan}}
			runtime := &Runtime{snapshot: stub, profile: test.profile, meta: requestMeta("test")}
			_, err := runtime.readMaterialSnapshot(t.Context(), "ses_1")
			if (err != nil) != test.wantErr {
				t.Fatalf("material snapshot error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestSessionColdReadBindsMaterialToOneMetadataProjection(t *testing.T) {
	stub := &snapshotBindingStub{
		sessions: []*protocol.Session{snapshotSession(1), snapshotSession(2), snapshotSession(2)},
		snapshot: &protocol.SessionSnapshot{},
	}
	runtime := &Runtime{snapshot: stub, meta: requestMeta("test")}
	snapshot, err := runtime.GetSession(t.Context(), "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Session.Revision != 2 || len(stub.snapshotRequests) != 2 || stub.sessionCalls != 3 {
		t.Fatalf(
			"cold read = revision %d, material calls %d, session calls %d; want 2/2/3",
			snapshot.Session.Revision, len(stub.snapshotRequests), stub.sessionCalls,
		)
	}
}

func TestSessionColdReadStopsWhenMetadataNeverStabilizes(t *testing.T) {
	stub := &snapshotBindingStub{
		sessionAt: func(call int) *protocol.Session { return snapshotSession(uint64(call)) },
		snapshot:  &protocol.SessionSnapshot{},
	}
	runtime := &Runtime{snapshot: stub, meta: requestMeta("test")}
	_, err := runtime.GetSession(t.Context(), "ses_1")
	if !errors.Is(err, agent.ErrDisconnected) || len(stub.snapshotRequests) != snapshotStabilityAttempts {
		t.Fatalf("cold read error = %v, material calls = %d", err, len(stub.snapshotRequests))
	}
}

func TestSessionMaterialSnapshotRejectsMissingResponses(t *testing.T) {
	stub := &snapshotBindingStub{}
	runtime := &Runtime{snapshot: stub, meta: requestMeta("test")}
	if _, err := runtime.readSession(t.Context(), "ses_1"); err == nil {
		t.Fatal("accepted a nil Session response")
	}
	if _, err := runtime.readMaterialSnapshot(t.Context(), "ses_1"); err == nil {
		t.Fatal("accepted a nil SessionSnapshot response")
	}
}

func TestSessionMaterialSnapshotCanonicalizesRunCreationOrder(t *testing.T) {
	created := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	finishedRun := func(id string, at time.Time) protocol.RunRef {
		return protocol.RunRef{
			RunSummary: protocol.RunSummary{
				ID: id, SessionID: "ses_1", Status: protocol.RunStatusFinished,
				Outcome: &protocol.RunOutcome{Type: protocol.OutcomeCompleted}, CreatedAt: at, FinishedAt: at,
			},
			ProtocolProfile: protocol.RunProtocolProfile{
				RequiredFeatures: []protocol.RunProtocolFeature{}, InterruptTypes: []protocol.InterruptType{},
			},
		}
	}
	projected, err := projectSnapshot(coldRead{
		session: *snapshotSession(1),
		runs: []protocol.RunRef{
			finishedRun("run_later", created.Add(time.Second)), finishedRun("run_earlier", created),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Runs) != 2 || projected.Runs[0].ID != "run_earlier" || projected.Runs[1].ID != "run_later" {
		t.Fatalf("run order = %+v", projected.Runs)
	}
}
