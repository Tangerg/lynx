package platform_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/platform"
)

var _ agent.DeploymentResolver = platform.Catalog{}

func TestCatalogResolvesOnlyExactDeploymentReferences(t *testing.T) {
	first := catalogDeployment(t, "test.writer", "1.0.0", "first")
	replacement := catalogDeployment(t, "test.writer", "1.0.0", "replacement")
	catalog, err := platform.NewCatalog(first, replacement)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []agent.Deployment{first, replacement} {
		got, err := catalog.Resolve(want.DeploymentRef())
		if err != nil || got.DeploymentRef() != want.DeploymentRef() {
			t.Fatalf("Resolve(%s) = %s, %v", want.DeploymentRef(), got.DeploymentRef(), err)
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
		t.Fatalf("same-name/version missing reference error = %v", err)
	}
	if _, err := catalog.Resolve(agent.DeploymentRef{}); !errors.Is(err, agent.ErrInvalidDeploymentRef) {
		t.Fatalf("invalid reference error = %v", err)
	}
}

func TestCatalogRejectsInvalidAndDuplicateBindings(t *testing.T) {
	deployment := catalogDeployment(t, "test.duplicate", "1.0.0", "only")
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
		catalogDeployment(t, "test.zebra", "1.0.0", "zebra"),
		catalogDeployment(t, "test.alpha", "1.10.0", "newer"),
		catalogDeployment(t, "test.alpha", "1.2.0", "older"),
	}
	catalog, err := platform.NewCatalog(deployments...)
	if err != nil {
		t.Fatal(err)
	}
	listed := catalog.Deployments()
	want := []string{"test.alpha@1.2.0", "test.alpha@1.10.0", "test.zebra@1.0.0"}
	got := make([]string, len(listed))
	for index, deployment := range listed {
		got[index] = deployment.DeploymentRef().Name() + "@" + deployment.DeploymentRef().Version()
	}
	if !slices.Equal(got, want) {
		t.Fatalf("catalog order = %v, want %v", got, want)
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
	deployment := catalogDeployment(t, "test.concurrent", "1.0.0", "stable")
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

func catalogDeployment(t *testing.T, name, version, configuration string) agent.Deployment {
	t.Helper()
	schema, err := agent.SchemaFor[struct{}]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: "Provide one immutable catalog test binding.", Version: version,
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
