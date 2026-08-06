package platform_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/platform"
)

var _ agent.DeploymentResolver = (*platform.Platform)(nil)

func TestPlatformDeployReplaceUndeployKeepsExactHistory(t *testing.T) {
	firstV1 := catalogDeployment(t, "test.writer", "1.0.0", "v1-first")
	v2 := catalogDeployment(t, "test.writer", "2.0.0", "v2")
	platformInstance, err := platform.New(v2, firstV1)
	if err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, firstV1.Reference(), v2.Reference())

	if err := platformInstance.Deploy(firstV1); err != nil {
		t.Fatalf("reapply exact Deployment: %v", err)
	}
	replacementV1 := catalogDeployment(t, "test.writer", "1.0.0", "v1-replacement")
	if err := platformInstance.Deploy(replacementV1); !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("conflicting Deploy error = %v", err)
	} else {
		var conflict *platform.DeploymentConflictError
		if !errors.As(err, &conflict) || conflict.Active != firstV1.Reference() ||
			conflict.Requested != replacementV1.Reference() {
			t.Fatalf("Deploy conflict = %#v", conflict)
		}
	}
	if err := platformInstance.Replace(replacementV1); err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, replacementV1.Reference(), v2.Reference())
	if _, err := platformInstance.Resolve(firstV1.Reference()); err != nil {
		t.Fatalf("replaced historical Deployment is not exact-resolvable: %v", err)
	}

	v3 := catalogDeployment(t, "test.writer", "3.0.0", "v3")
	if err := platformInstance.Replace(v3); !errors.Is(err, platform.ErrDeploymentNotActive) {
		t.Fatalf("Replace into new version error = %v", err)
	}
	if err := platformInstance.Undeploy(firstV1.Reference()); !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("stale Undeploy error = %v", err)
	}
	if err := platformInstance.Undeploy(replacementV1.Reference()); err != nil {
		t.Fatal(err)
	}
	assertActiveReferences(t, platformInstance, v2.Reference())
	if _, err := platformInstance.Resolve(replacementV1.Reference()); err != nil {
		t.Fatalf("undeployed historical Deployment is not exact-resolvable: %v", err)
	}
	if err := platformInstance.Undeploy(replacementV1.Reference()); !errors.Is(err, platform.ErrDeploymentNotActive) {
		t.Fatalf("repeated Undeploy error = %v", err)
	}
	if got := len(platformInstance.Catalog().Deployments()); got != 3 {
		t.Fatalf("historical Catalog size = %d, want 3", got)
	}
}

func TestPlatformInitialConfigurationRejectsConflictingVersionSlot(t *testing.T) {
	first := catalogDeployment(t, "test.conflict", "1.0.0", "first")
	second := catalogDeployment(t, "test.conflict", "1.0.0", "second")
	instance, err := platform.New(first, second)
	if instance != nil || !errors.Is(err, platform.ErrDeploymentConflict) {
		t.Fatalf("New conflicting Platform = %#v, %v", instance, err)
	}
}

func TestPlatformInstancesDoNotShareDeploymentState(t *testing.T) {
	deployment := catalogDeployment(t, "test.isolated", "1.0.0", "only-left")
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
	if _, err := right.Resolve(deployment.Reference()); !errors.Is(err, platform.ErrDeploymentNotFound) {
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
		version := fmt.Sprintf("1.0.%d", index)
		initial[index] = catalogDeployment(t, "test.concurrent_platform", version, "initial:"+version)
		replacements[index] = catalogDeployment(t, "test.concurrent_platform", version, "replacement:"+version)
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
					if _, err := instance.Resolve(candidate.Reference()); err != nil {
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
	deployment := catalogDeployment(t, "test.zero_platform", "1.0.0", "zero")
	if err := zeroPlatform.Deploy(deployment); err != nil {
		t.Fatalf("zero-value Deploy: %v", err)
	}
	if _, err := zeroPlatform.Resolve(deployment.Reference()); err != nil {
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
		if candidate.Reference() != want[index] {
			t.Fatalf("active Deployment %d = %s, want %s", index, candidate.Reference(), want[index])
		}
	}
}
