package platform_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/platform"
)

func TestSelectDeploymentUsesOnlyStableActiveCandidateSnapshot(t *testing.T) {
	first := catalogDeployment(t, "test.selector", "1.0.0", "first")
	second := catalogDeployment(t, "test.selector", "2.0.0", "second")
	instance, err := platform.New(second, first)
	if err != nil {
		t.Fatal(err)
	}
	candidates := instance.DeploymentCandidates()
	if len(candidates) != 2 || candidates[0].Reference() != first.Reference() ||
		candidates[1].Reference() != second.Reference() ||
		candidates[0].Descriptor().Digest() != first.Descriptor().Digest() {
		t.Fatalf("DeploymentCandidates = %#v", candidates)
	}
	candidates[0] = platform.DeploymentCandidate{}
	selector := platform.DeploymentSelectorFunc(func(
		_ context.Context,
		offered []platform.DeploymentCandidate,
	) (agent.DeploymentRef, error) {
		if len(offered) != 2 || offered[0].Reference() != first.Reference() {
			t.Fatalf("offered candidates = %#v", offered)
		}
		return offered[1].Reference(), nil
	})
	selected, err := instance.SelectDeployment(context.Background(), selector)
	if err != nil || selected.Reference() != second.Reference() {
		t.Fatalf("SelectDeployment = %s, %v", selected.Reference(), err)
	}
}

func TestSelectDeploymentDoesNotFollowConcurrentReplacement(t *testing.T) {
	first := catalogDeployment(t, "test.snapshot_selection", "1.0.0", "first")
	replacement := catalogDeployment(t, "test.snapshot_selection", "1.0.0", "replacement")
	instance, err := platform.New(first)
	if err != nil {
		t.Fatal(err)
	}
	captured := make(chan struct{})
	release := make(chan struct{})
	result := make(chan selectionResult, 1)
	go func() {
		selected, err := instance.SelectDeployment(context.Background(), platform.DeploymentSelectorFunc(func(
			_ context.Context,
			candidates []platform.DeploymentCandidate,
		) (agent.DeploymentRef, error) {
			close(captured)
			<-release
			return candidates[0].Reference(), nil
		}))
		result <- selectionResult{deployment: selected, err: err}
	}()
	<-captured
	if err := instance.Replace(replacement); err != nil {
		t.Fatal(err)
	}
	close(release)
	selected := <-result
	if selected.err != nil || selected.deployment.Reference() != first.Reference() {
		t.Fatalf("selection followed replacement: %s, %v", selected.deployment.Reference(), selected.err)
	}
	if active := instance.DeploymentCandidates(); len(active) != 1 || active[0].Reference() != replacement.Reference() {
		t.Fatalf("active replacement = %#v", active)
	}
}

func TestSelectDeploymentRejectsHistoricalOrMalformedSelection(t *testing.T) {
	first := catalogDeployment(t, "test.selection_contract", "1.0.0", "first")
	replacement := catalogDeployment(t, "test.selection_contract", "1.0.0", "replacement")
	instance, err := platform.New(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Replace(replacement); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		selection agent.DeploymentRef
	}{
		{name: "historical", selection: first.Reference()},
		{name: "invalid", selection: agent.DeploymentRef{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := instance.SelectDeployment(context.Background(), platform.DeploymentSelectorFunc(func(
				context.Context,
				[]platform.DeploymentCandidate,
			) (agent.DeploymentRef, error) {
				return test.selection, nil
			}))
			if !errors.Is(err, platform.ErrInvalidDeploymentSelection) {
				t.Fatalf("selection error = %v", err)
			}
		})
	}
}

func TestSelectDeploymentContainsSelectorFailureAndPanic(t *testing.T) {
	deployment := catalogDeployment(t, "test.selection_failure", "1.0.0", "only")
	instance, err := platform.New(deployment)
	if err != nil {
		t.Fatal(err)
	}
	wantFailure := errors.New("selector unavailable")
	for _, test := range []struct {
		name     string
		selector platform.DeploymentSelector
		want     error
	}{
		{
			name: "failure",
			selector: platform.DeploymentSelectorFunc(func(context.Context, []platform.DeploymentCandidate) (agent.DeploymentRef, error) {
				return agent.DeploymentRef{}, wantFailure
			}),
			want: wantFailure,
		},
		{
			name: "panic",
			selector: platform.DeploymentSelectorFunc(func(context.Context, []platform.DeploymentCandidate) (agent.DeploymentRef, error) {
				panic("broken selector")
			}),
			want: platform.ErrInvalidDeploymentSelection,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := instance.SelectDeployment(context.Background(), test.selector); !errors.Is(err, test.want) {
				t.Fatalf("SelectDeployment error = %v", err)
			}
		})
	}
}

func TestSelectDeploymentRejectsNilAndEmptyCandidateSet(t *testing.T) {
	empty, err := platform.New()
	if err != nil {
		t.Fatal(err)
	}
	called := false
	selector := platform.DeploymentSelectorFunc(func(context.Context, []platform.DeploymentCandidate) (agent.DeploymentRef, error) {
		called = true
		return agent.DeploymentRef{}, nil
	})
	if _, err := empty.SelectDeployment(context.Background(), selector); !errors.Is(err, platform.ErrNoDeploymentCandidate) || called {
		t.Fatalf("empty selection = called %t, error %v", called, err)
	}
	var typedNil platform.DeploymentSelectorFunc
	if _, err := empty.SelectDeployment(context.Background(), typedNil); !errors.Is(err, platform.ErrNilDeploymentSelector) {
		t.Fatalf("typed-nil selector error = %v", err)
	}
	var zero platform.Platform
	if _, err := zero.SelectDeployment(context.Background(), selector); !errors.Is(err, platform.ErrNoDeploymentCandidate) || called {
		t.Fatalf("zero Platform selection error = %v", err)
	}
}

func TestDeploymentCandidatesExcludeUndeployedHistory(t *testing.T) {
	first := catalogDeployment(t, "test.candidate_history", "1.0.0", "first")
	second := catalogDeployment(t, "test.candidate_history", "2.0.0", "second")
	instance, err := platform.New(first, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Undeploy(first.Reference()); err != nil {
		t.Fatal(err)
	}
	candidates := instance.DeploymentCandidates()
	got := make([]agent.DeploymentRef, len(candidates))
	for index, candidate := range candidates {
		got[index] = candidate.Reference()
	}
	if !slices.Equal(got, []agent.DeploymentRef{second.Reference()}) {
		t.Fatalf("active candidates = %v", got)
	}
	if _, err := instance.Resolve(first.Reference()); err != nil {
		t.Fatalf("undeployed history is not exact-resolvable: %v", err)
	}
}

type selectionResult struct {
	deployment agent.Deployment
	err        error
}
