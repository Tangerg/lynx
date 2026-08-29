package platform_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/platform"
)

var _ agent.DeploymentResolver = platform.Catalog{}

func TestCatalogResolvesOnlyExactDeploymentReferences(t *testing.T) {
	first := catalogDeployment(t, "test.writer", "first")
	replacement := catalogDeployment(t, "test.writer", "replacement")
	catalog, err := platform.NewCatalog(first, replacement)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []agent.Deployment{first, replacement} {
		got, resolveErr := catalog.Resolve(want.DeploymentRef())
		if resolveErr != nil || got.DeploymentRef() != want.DeploymentRef() {
			t.Fatalf("Resolve(%s) = %s, %v", want.DeploymentRef(), got.DeploymentRef(), resolveErr)
		}
	}
	missingReference, err := agent.NewDeploymentRef(
		first.Descriptor(), first.DeploymentRef().ImplementationDigest(),
		agent.ComputeDigest([]byte("missing configuration")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Resolve(missingReference); !errors.Is(err, platform.ErrDeploymentNotFound) {
		t.Fatalf("same-name missing reference error = %v", err)
	}
	if _, err := catalog.Resolve(agent.DeploymentRef{}); !errors.Is(err, agent.ErrInvalidDeploymentRef) {
		t.Fatalf("invalid reference error = %v", err)
	}
}

func TestCatalogRejectsInvalidAndDuplicateBindings(t *testing.T) {
	deployment := catalogDeployment(t, "test.duplicate", "only")
	for _, test := range []struct {
		name        string
		deployments []agent.Deployment
	}{
		{name: "invalid", deployments: []agent.Deployment{{}}},
		{name: "duplicate exact reference", deployments: []agent.Deployment{deployment, deployment}},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, err := platform.NewCatalog(test.deployments...)
			if !errors.Is(err, platform.ErrInvalidCatalog) || len(catalog.Deployments()) != 0 {
				t.Fatalf("NewCatalog() = %#v, %v", catalog.Deployments(), err)
			}
		})
	}
}

func TestCatalogEnumerationIsStableAndOwnershipIsolated(t *testing.T) {
	deployments := []agent.Deployment{
		catalogDeployment(t, "test.zebra", "zebra"),
		catalogDeployment(t, "test.alpha", "second"),
		catalogDeployment(t, "test.alpha", "first"),
	}
	catalog, err := platform.NewCatalog(deployments...)
	if err != nil {
		t.Fatal(err)
	}
	listed := catalog.Deployments()
	if !slices.IsSortedFunc(listed, func(left, right agent.Deployment) int {
		leftRef, rightRef := left.DeploymentRef(), right.DeploymentRef()
		if leftRef.Name() < rightRef.Name() {
			return -1
		}
		if leftRef.Name() > rightRef.Name() {
			return 1
		}
		return strings.Compare(leftRef.Digest().String(), rightRef.Digest().String())
	}) {
		t.Fatalf("catalog order is unstable: %v", listed)
	}
	listed[0] = agent.Deployment{}
	if again := catalog.Deployments(); len(again) != 3 || !again[0].Valid() {
		t.Fatalf("caller mutated catalog enumeration = %#v", again)
	}
}

func TestCatalogZeroValueIsEmptyAndConcurrentSafe(t *testing.T) {
	var empty platform.Catalog
	if listed := empty.Deployments(); len(listed) != 0 {
		t.Fatalf("zero Catalog deployments = %#v", listed)
	}
	deployment := catalogDeployment(t, "test.concurrent", "stable")
	catalog, err := platform.NewCatalog(deployment)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			for range 100 {
				resolved, err := catalog.Resolve(deployment.DeploymentRef())
				if err != nil || resolved.DeploymentRef() != deployment.DeploymentRef() {
					t.Errorf("concurrent Resolve() = %s, %v", resolved.DeploymentRef(), err)
					return
				}
				listed := catalog.Deployments()
				if len(listed) != 1 || listed[0].DeploymentRef() != deployment.DeploymentRef() {
					t.Errorf("concurrent Deployments() = %#v", listed)
					return
				}
			}
		})
	}
	group.Wait()
}

type catalogDefinition struct {
	descriptor agent.Descriptor
}

func (c catalogDefinition) Descriptor() agent.Descriptor { return c.descriptor }

func (catalogDefinition) Start(agent.Input) (agent.Execution, error) {
	return nil, errors.New("catalog fixture is not executable")
}

func (catalogDefinition) Restore(agent.ExecutionState) (agent.Execution, error) {
	return nil, errors.New("catalog fixture is not restorable")
}

type catalogDispatcher struct{}

func (catalogDispatcher) Dispatch(
	context.Context,
	agent.EffectRequest,
	agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, errors.New("catalog fixture has no effects")
}

func (catalogDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

func catalogDeployment(t *testing.T, name, configuration string) agent.Deployment {
	t.Helper()
	schema, err := agent.SchemaFor[struct{}]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: "Provide one immutable catalog test binding.",
		InputSchema: schema, OutputSchema: schema,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: catalogDefinition{descriptor: descriptor}, Dispatcher: catalogDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("catalog implementation:" + name)),
		ConfigurationDigest:  agent.ComputeDigest([]byte(configuration)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}
