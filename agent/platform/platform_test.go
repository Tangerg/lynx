package platform_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/platform"
)

var _ agent.DeploymentResolver = (*platform.Platform)(nil)

func TestPlatformDeployReplaceUndeployKeepsExactHistory(t *testing.T) {
	firstWriter := catalogDeployment(t, "test.writer", "first")
	reader := catalogDeployment(t, "test.reader", "reader")
	platformInstance, err := platform.New(reader, firstWriter)
	if err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, reader.DeploymentRef(), firstWriter.DeploymentRef())

	if err := platformInstance.Deploy(firstWriter); err != nil {
		t.Fatalf("reapply exact Deployment: %v", err)
	}
	replacementWriter := catalogDeployment(t, "test.writer", "replacement")
	if err := platformInstance.Deploy(replacementWriter); !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("conflicting Deploy error = %v", err)
	} else {
		var conflict *platform.DeploymentConflictError
		if !errors.As(err, &conflict) || conflict.Active != firstWriter.DeploymentRef() ||
			conflict.Requested != replacementWriter.DeploymentRef() {
			t.Fatalf("Deploy conflict = %#v", conflict)
		}
	}
	if err := platformInstance.Replace(replacementWriter); err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, reader.DeploymentRef(), replacementWriter.DeploymentRef())
	if _, err := platformInstance.Resolve(firstWriter.DeploymentRef()); err != nil {
		t.Fatalf("replaced historical Deployment is not exact-resolvable: %v", err)
	}

	unseen := catalogDeployment(t, "test.unseen", "unseen")
	if err := platformInstance.Replace(unseen); !errors.Is(err, platform.ErrDeploymentNotActive) {
		t.Fatalf("Replace into inactive name error = %v", err)
	}
	if err := platformInstance.Undeploy(firstWriter.DeploymentRef()); !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("stale Undeploy error = %v", err)
	}
	if err := platformInstance.Undeploy(replacementWriter.DeploymentRef()); err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, reader.DeploymentRef())
	if _, err := platformInstance.Resolve(replacementWriter.DeploymentRef()); err != nil {
		t.Fatalf("undeployed historical Deployment is not exact-resolvable: %v", err)
	}
	if err := platformInstance.Undeploy(replacementWriter.DeploymentRef()); !errors.Is(err, platform.ErrDeploymentNotActive) {
		t.Fatalf("repeated Undeploy error = %v", err)
	}
	if got := len(platformInstance.Catalog().Deployments()); got != 3 {
		t.Fatalf("historical Catalog size = %d, want 3", got)
	}
}

func TestPlatformInitialConfigurationRejectsConflictingNameSlot(t *testing.T) {
	first := catalogDeployment(t, "test.conflict", "first")
	second := catalogDeployment(t, "test.conflict", "second")
	instance, err := platform.New(first, second)
	if instance != nil || !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("New conflicting Platform = %#v, %v", instance, err)
	}
}

func TestPlatformInstancesDoNotShareDeploymentState(t *testing.T) {
	deployment := catalogDeployment(t, "test.isolated", "only-left")
	left, err := platform.New()
	if err != nil {
		t.Fatal(err)
	}
	right, err := platform.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := left.Deploy(deployment); err != nil {
		t.Fatal(err)
	}
	if len(left.DeploymentCandidates()) != 1 || len(right.DeploymentCandidates()) != 0 {
		t.Fatalf("Platform state leaked: left=%d right=%d", len(left.DeploymentCandidates()), len(right.DeploymentCandidates()))
	}
	if _, err := right.Resolve(deployment.DeploymentRef()); !errors.Is(err, platform.ErrDeploymentNotFound) {
		t.Fatalf("right Platform Resolve error = %v", err)
	}
}

func TestPlatformSerializesConcurrentChangesAndPublishesConsistentSnapshots(t *testing.T) {
	instance, err := platform.New()
	if err != nil {
		t.Fatal(err)
	}
	const count = 24
	initial := make([]agent.Deployment, count)
	replacements := make([]agent.Deployment, count)
	for index := range count {
		name := fmt.Sprintf("test.concurrent_platform.slot_%d", index)
		initial[index] = catalogDeployment(t, name, "initial:"+name)
		replacements[index] = catalogDeployment(t, name, "replacement:"+name)
	}
	var group sync.WaitGroup
	for index := range count {
		group.Go(func() {
			if err := instance.Deploy(initial[index]); err != nil {
				t.Errorf("Deploy %d: %v", index, err)
			}
		})
	}
	group.Wait()
	for range 16 {
		group.Go(func() {
			for range 100 {
				candidates := instance.DeploymentCandidates()
				for _, candidate := range candidates {
					if _, err := instance.Resolve(candidate.DeploymentRef()); err != nil {
						t.Errorf("active Deployment missing from Catalog: %v", err)
						return
					}
				}
			}
		})
	}
	for index := range count {
		group.Go(func() {
			if err := instance.Replace(replacements[index]); err != nil {
				t.Errorf("Replace %d: %v", index, err)
			}
		})
	}
	group.Wait()
	if got := len(instance.DeploymentCandidates()); got != count {
		t.Fatalf("active Deployments = %d, want %d", got, count)
	}
	if got := len(instance.Catalog().Deployments()); got != count*2 {
		t.Fatalf("historical Deployments = %d, want %d", got, count*2)
	}
}

func TestPlatformZeroValueIsUsableAndNilReceiverIsRejected(t *testing.T) {
	var nilPlatform *platform.Platform
	var zeroPlatform platform.Platform
	if err := nilPlatform.Deploy(agent.Deployment{}); !errors.Is(err, platform.ErrNilPlatform) {
		t.Fatalf("nil Deploy error = %v", err)
	}
	if _, err := nilPlatform.Resolve(agent.DeploymentRef{}); !errors.Is(err, platform.ErrNilPlatform) {
		t.Fatalf("nil Resolve error = %v", err)
	}
	deployment := catalogDeployment(t, "test.zero_platform", "zero")
	if err := zeroPlatform.Deploy(deployment); err != nil {
		t.Fatalf("zero-value Deploy: %v", err)
	}
	if _, err := zeroPlatform.Resolve(deployment.DeploymentRef()); err != nil {
		t.Fatalf("zero-value Resolve: %v", err)
	}
	instance, err := platform.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Deploy(agent.Deployment{}); !errors.Is(err, agent.ErrInvalidDeployment) {
		t.Fatalf("zero Deployment error = %v", err)
	}
	if err := instance.Undeploy(agent.DeploymentRef{}); !errors.Is(err, agent.ErrInvalidDeploymentRef) {
		t.Fatalf("zero DeploymentRef error = %v", err)
	}
}

func assertActiveReferences(t *testing.T, instance *platform.Platform, want ...agent.DeploymentRef) {
	t.Helper()
	candidates := instance.DeploymentCandidates()
	if len(candidates) != len(want) {
		t.Fatalf("active Deployment count = %d, want %d", len(candidates), len(want))
	}
	for index, candidate := range candidates {
		if candidate.DeploymentRef() != want[index] {
			t.Fatalf("active Deployment %d = %s, want %s", index, candidate.DeploymentRef(), want[index])
		}
	}
}
