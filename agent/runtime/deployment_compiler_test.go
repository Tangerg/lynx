package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/goap"
)

type mutableDeploymentAction struct {
	metadata core.ActionMetadata
	run      func(*core.ProcessContext) core.ActionStatus
}

type panickingMetadataAction struct{ cause error }

func (a panickingMetadataAction) Metadata() core.ActionMetadata { panic(a.cause) }
func (panickingMetadataAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func (a *mutableDeploymentAction) Metadata() core.ActionMetadata { return a.metadata }

func (a *mutableDeploymentAction) Execute(_ context.Context, process *core.ProcessContext) (core.ActionStatus, error) {
	if a.run == nil {
		return core.ActionSucceeded, nil
	}
	return a.run(process), nil
}

type mutableDeploymentCondition struct {
	name string
	cost float64
}

type deploymentGoldenInput struct {
	Topic string `json:"topic"`
}

type deploymentGoldenStuckPolicy struct{}

func (deploymentGoldenStuckPolicy) Recover(context.Context, core.ProcessView, core.BlackboardWriter) (core.StuckResult, error) {
	return core.StuckResult{Decision: core.StuckStop}, nil
}

func (c *mutableDeploymentCondition) Name() string  { return c.name }
func (c *mutableDeploymentCondition) Cost() float64 { return c.cost }
func (c *mutableDeploymentCondition) Evaluate(context.Context, *core.ConditionEnv) core.Truth {
	return core.True
}

func TestCompileDeploymentFreezesPlannerDefinition(t *testing.T) {
	action := &mutableDeploymentAction{metadata: deploymentActionMetadata("finish")}
	condition := &mutableDeploymentCondition{name: "ready", cost: 2.5}
	pre := []string{"finish"}
	goal := core.NewGoal(core.GoalConfig{
		Name: "complete", Preconditions: pre,
	})
	actions := []core.Action{action}
	goals := []*core.Goal{goal}
	conditions := []core.Condition{condition}
	snapshotState := []core.Binding{core.NewBinding[deploymentGoldenInput]("draft_state")}
	config := core.AgentConfig{
		Name:          "writer",
		Description:   "original",
		Version:       "1.2.3",
		PlannerName:   "goap",
		Actions:       actions,
		Goals:         goals,
		Conditions:    conditions,
		SnapshotState: snapshotState,
	}
	source := core.NewAgent(config)

	compiled, err := (deploymentCompiler{}).compile(source)
	if err != nil {
		t.Fatal(err)
	}

	// Mutate every caller-owned input after construction or compilation. Agent
	// owns its declaration; Deployment additionally freezes SPI metadata.
	config.Description = "mutated"
	config.Version = "2.0.0"
	actions[0] = nil
	goals[0] = nil
	conditions[0] = nil
	snapshotState[0].Name = "mutated"
	pre[0] = "mutated"
	action.metadata.Inputs[0].Name = "mutated"
	action.metadata.Effects["finish"] = core.False
	action.metadata.ToolGroups[0] = "mutated"
	condition.name = "mutated"
	condition.cost = 99

	frozen := compiled.agent
	if frozen.Description() != "original" || frozen.Version() != "1.2.3" {
		t.Fatalf("frozen scalar definition = description %q, version %q", frozen.Description(), frozen.Version())
	}
	if len(frozen.Actions()) != 1 {
		t.Fatalf("frozen actions = %d, want 1", len(frozen.Actions()))
	}
	metadata := frozen.Actions()[0].Metadata()
	if metadata.Inputs[0].Name != "input" || metadata.Effects["finish"] != core.True {
		t.Fatalf("frozen metadata was mutated: %#v", metadata)
	}
	if metadata.ToolGroups[0] != "filesystem" {
		t.Fatalf("frozen tool groups = %v", metadata.ToolGroups)
	}
	frozenGoal := frozen.Goals()[0]
	if frozenGoal.RequiredConditions()[0] != "finish" {
		t.Fatalf("frozen goal was mutated: %#v", frozenGoal)
	}
	if frozen.Conditions()[0].Name() != "ready" || frozen.Conditions()[0].Cost() != 2.5 {
		t.Fatalf("frozen condition = %q/%v", frozen.Conditions()[0].Name(), frozen.Conditions()[0].Cost())
	}
	if state := frozen.SnapshotState(); len(state) != 1 || state[0].Name != "draft_state" {
		t.Fatalf("frozen snapshot state = %#v", state)
	}

	// Metadata access itself must remain defensive.
	metadata.Effects["finish"] = core.False
	metadata.ToolGroups[0] = "mutated"
	again := frozen.Actions()[0].Metadata()
	if again.Effects["finish"] != core.True || again.ToolGroups[0] != "filesystem" {
		t.Fatalf("metadata accessor leaked deployment state: %#v", again)
	}
}

func TestCompileDeploymentRejectsInvalidFrozenDefinition(t *testing.T) {
	source := core.NewAgent(core.AgentConfig{
		Name:    "invalid-frozen",
		Actions: []core.Action{&mutableDeploymentAction{metadata: core.ActionMetadata{Name: ""}}},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	})
	if _, err := (deploymentCompiler{}).compile(source); err == nil || !strings.Contains(err.Error(), "empty name") {
		t.Fatalf("compile error = %v, want frozen-definition validation", err)
	}
}

func TestCompileDeploymentContainsMetadataPanic(t *testing.T) {
	cause := errors.New("metadata sentinel")
	source := core.NewAgent(core.AgentConfig{
		Name:    "panicking-metadata",
		Actions: []core.Action{panickingMetadataAction{cause: cause}},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	})
	_, err := (deploymentCompiler{}).compile(source)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "snapshot agent definition panicked") {
		t.Fatalf("compile error = %v, want wrapped metadata panic", err)
	}
}

func TestCompileDeploymentCanonicalizesDefaultPlanner(t *testing.T) {
	implicit := deploymentFixture("canonical-planner", core.ConditionSet{"finish": core.True}, nil)
	implicit = reconfigureAgent(implicit, func(config *core.AgentConfig) {
		config.PlannerName = ""
	})
	explicit := reconfigureAgent(implicit, func(config *core.AgentConfig) {
		config.PlannerName = planning.DefaultPlannerName
	})

	implicitDeployment, err := (deploymentCompiler{}).compile(implicit)
	if err != nil {
		t.Fatal(err)
	}
	explicitDeployment, err := (deploymentCompiler{}).compile(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if implicitDeployment.ref != explicitDeployment.ref {
		t.Fatalf("implicit deployment = %s, explicit = %s", implicitDeployment.ref, explicitDeployment.ref)
	}
}

func TestCompiledDefinitionDigestIsDeterministicAndSemantic(t *testing.T) {
	firstEffects := core.ConditionSet{"alpha": core.True, "beta": core.False, "finish": core.True, "ready": core.True}
	secondEffects := core.ConditionSet{}
	secondEffects["beta"] = core.False
	secondEffects["alpha"] = core.True
	secondEffects["ready"] = core.True
	secondEffects["finish"] = core.True

	first := deploymentFixtureWith("writer", firstEffects, func(*core.ProcessContext) core.ActionStatus {
		return core.ActionSucceeded
	}, []string{"finish", "ready"}, "goap")
	second := deploymentFixtureWith("writer", secondEffects, func(*core.ProcessContext) core.ActionStatus {
		return core.ActionFailed
	}, []string{"ready", "finish", "finish"}, "") // empty and explicit goap are semantically equal

	compiledFirst, err := (deploymentCompiler{}).compile(first)
	if err != nil {
		t.Fatal(err)
	}
	compiledSecond, err := (deploymentCompiler{}).compile(second)
	if err != nil {
		t.Fatal(err)
	}
	if compiledFirst.ref.Digest != compiledSecond.ref.Digest {
		t.Fatalf("equivalent declarations have different digests:\n%s\n%s", compiledFirst.ref.Digest, compiledSecond.ref.Digest)
	}
	if !json.Valid(compiledFirst.definition) {
		t.Fatalf("canonical definition is not JSON: %s", compiledFirst.definition)
	}

	changed := deploymentFixture("writer", core.ConditionSet{"alpha": core.False, "beta": core.False, "finish": core.True}, nil)
	compiledChanged, err := (deploymentCompiler{}).compile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if compiledChanged.ref.Digest == compiledFirst.ref.Digest {
		t.Fatal("declarative effect change did not change digest")
	}

	// Different function bodies intentionally do not affect the digest. This
	// pins the honest contract: callers must change semantic Version when
	// executable behavior changes.
	if compiledFirst.ref.Digest != compiledSecond.ref.Digest {
		t.Fatal("function implementation unexpectedly entered deterministic digest")
	}
}

func TestCompiledDefinitionMatchesGolden(t *testing.T) {
	actionMetadata := core.ActionMetadata{
		Name:        "research",
		Description: "collect evidence",
		Inputs: []core.Binding{
			{Name: "", Type: "example.Topic"},
			{Name: "context", Type: "example.Context"},
		},
		Outputs:           []core.Binding{{Name: "report", Type: "example.Report"}},
		Preconditions:     core.ConditionSet{"authorized": core.True, "blocked": core.False, "reviewed": core.Unknown},
		Effects:           core.ConditionSet{"complete": core.True, "stale": core.False},
		Repeatable:        true,
		ToolGroups:        []string{"search", "memory"},
		Cost:              core.FixedScore(2.5),
		Value:             core.FixedScore(7),
		ClearWorkingState: true,
	}
	source := core.NewAgent(core.AgentConfig{
		Name:        "golden-agent",
		Description: "canonical definition fixture",
		Version:     "2.3.4",
		PlannerName: "htn",
		StuckPolicy: deploymentGoldenStuckPolicy{},
		Actions:     []core.Action{&mutableDeploymentAction{metadata: actionMetadata}},
		Goals: []*core.Goal{
			core.NewGoal(core.GoalConfig{Name: "publish-report", Description: "publish the researched report", Preconditions: []string{"complete", "authorized", "complete"}, Inputs: []core.Binding{{Name: "report", Type: "example.Report"}}, Value: core.FixedScore(11)}),
		},
		Conditions:    []core.Condition{&mutableDeploymentCondition{name: "authorized", cost: 1.25}},
		SnapshotState: []core.Binding{core.NewBinding[deploymentGoldenInput]("draft_state")},
	})

	compiled, err := (deploymentCompiler{}).compile(source)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/deployment_definition.golden.json")
	if err != nil {
		t.Fatalf("read deployment definition golden: %v\ncanonical definition:\n%s", err, compiled.definition)
	}
	if !bytes.Equal(compiled.definition, bytes.TrimSpace(want)) {
		t.Fatalf("canonical definition changed\nwant:\n%s\ngot:\n%s", bytes.TrimSpace(want), compiled.definition)
	}
	const wantDigest = "1a526fbeaef1e5ec8ba948be7dc3fb91e8416e6d55084a9d4cd902a8e8d07173"
	if compiled.ref.Digest != wantDigest {
		t.Fatalf("definition digest = %s, want %s", compiled.ref.Digest, wantDigest)
	}
}

func TestCanonicalDefinitionFieldInventory(t *testing.T) {
	// This test is intentionally explicit. A new exported declaration field can
	// affect planning, tool exposure, or snapshot restore identity; allowing it to
	// bypass the digest silently is more dangerous than making the author
	// classify it as canonical data or implementation identity.
	assertExportedFields(t, reflect.TypeFor[core.AgentConfig](), []string{
		"Name", "Description", "Version", "StuckPolicy", "Actions", "Goals", "Conditions", "SnapshotState", "PlannerName",
	})
	assertExportedFields(t, reflect.TypeFor[core.Agent](), nil)
	assertExportedFields(t, reflect.TypeFor[core.ActionMetadata](), []string{
		"Name", "Description", "Inputs", "Outputs", "Preconditions", "Effects", "Repeatable", "ToolGroups", "Cost", "Value", "ClearWorkingState",
	})
	assertExportedFields(t, reflect.TypeFor[core.GoalConfig](), []string{
		"Name", "Description", "Preconditions", "Inputs", "Value",
	})
	assertExportedFields(t, reflect.TypeFor[core.Goal](), nil)
	assertExportedFields(t, reflect.TypeFor[core.Binding](), []string{
		"Name", "Type",
	})
}

func assertExportedFields(t *testing.T, typeOf reflect.Type, want []string) {
	t.Helper()
	var got []string
	for i := range typeOf.NumField() {
		field := typeOf.Field(i)
		if field.IsExported() {
			got = append(got, field.Name)
		}
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s exported fields = %v, want %v; review canonical definition coverage before updating this inventory", typeOf, got, want)
	}
}

func TestAgentRegistryReturnsStableImmutableDeployments(t *testing.T) {
	registry := newDeploymentRegistry()
	for _, name := range []string{"zebra", "alpha"} {
		deployment, err := (deploymentCompiler{}).compile(deploymentFixture(name, core.ConditionSet{"finish": core.True}, nil))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := registry.activate(deployment, false); err != nil {
			t.Fatal(err)
		}
	}

	listed := registry.listActive()
	if len(listed) != 2 || listed[0].Ref().Name != "alpha" || listed[1].Ref().Name != "zebra" {
		t.Fatalf("registry order = %#v", []string{listed[0].Ref().Name, listed[1].Ref().Name})
	}
	definition := listed[0].Descriptor()
	actions := definition.Actions()
	goals := definition.Goals()
	pre := goals[0].RequiredConditions()
	actions[0] = core.ActionDescriptor{}
	goals[0] = core.GoalDescriptor{}
	pre[0] = "mutated"

	again := listed[0].Descriptor()
	if again.Description() != "fixture" ||
		again.Actions()[0].Name() == "" ||
		again.Goals()[0].Name() == "" ||
		again.Goals()[0].RequiredConditions()[0] != "finish" {
		t.Fatalf("deployment definition leaked mutation: %#v", again)
	}
}

func TestAgentRegistryRetainsHistoricalDefinitions(t *testing.T) {
	registry := newDeploymentRegistry()
	first, err := (deploymentCompiler{}).compile(deploymentFixture("writer", core.ConditionSet{"finish": core.True}, nil))
	if err != nil {
		t.Fatal(err)
	}
	second, err := (deploymentCompiler{}).compile(deploymentFixture("writer", core.ConditionSet{"finish": core.True, "replacement": core.True}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.activate(first, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.activate(second, true); err != nil {
		t.Fatal(err)
	}

	active, ok := registry.activeDeployment("writer")
	if !ok || active.agent.Actions()[0].Metadata().Effects["replacement"] != core.True {
		t.Fatalf("active deployment = %#v, %v", active, ok)
	}
	if historical, ok := registry.lookup(first.Ref()); !ok || historical != first {
		t.Fatalf("first historical deployment = %#v, %v", historical, ok)
	}
	if historical, ok := registry.lookup(second.Ref()); !ok || historical != second {
		t.Fatalf("second historical deployment = %#v, %v", historical, ok)
	}

	if _, err := registry.unregister("writer"); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.activeDeployment("writer"); ok {
		t.Fatal("unregister left deployment active")
	}
	if _, ok := registry.lookup(first.Ref()); !ok {
		t.Fatal("unregister destroyed historical deployment")
	}
}

func TestEngineDeploymentConflictReplaceAndHistoricalLookup(t *testing.T) {
	var lifecycle []event.Event
	listener := event.NewNamedListener("deployment-lifecycle", func(_ context.Context, published event.Event) {
		lifecycle = append(lifecycle, published)
	})
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner(), listener}})
	first := deploymentFixture("writer", core.ConditionSet{"finish": core.True}, nil)
	second := reconfigureAgent(deploymentFixture("writer", core.ConditionSet{"finish": core.True}, nil), func(config *core.AgentConfig) {
		config.Description = "replacement"
	})

	firstDeployment, err := engine.Deploy(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	idempotent, err := engine.Deploy(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	if idempotent != firstDeployment {
		t.Fatal("idempotent deploy returned a different handle")
	}

	_, err = engine.Deploy(t.Context(), second)
	if !errors.Is(err, ErrDeploymentConflict) {
		t.Fatalf("Deploy error = %v, want ErrDeploymentConflict", err)
	}
	var conflict *DeploymentConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Deploy error = %T, want DeploymentConflictError", err)
	}
	if conflict.Active != firstDeployment.Ref() || conflict.Candidate == firstDeployment.Ref() {
		t.Fatalf("conflict = %#v", conflict)
	}
	if active, ok := engine.ActiveDeployment("writer"); !ok || active != firstDeployment {
		t.Fatalf("active deployment = %#v, %v; want first", active, ok)
	}

	secondDeployment, err := engine.Replace(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	if active, ok := engine.ActiveDeployment("writer"); !ok || active != secondDeployment {
		t.Fatalf("active deployment = %#v, %v; want replacement", active, ok)
	}
	if historical, ok := engine.Deployment(firstDeployment.Ref()); !ok || historical != firstDeployment {
		t.Fatalf("historical first deployment = %#v, %v", historical, ok)
	}

	if err := engine.Undeploy(t.Context(), "writer"); err != nil {
		t.Fatal(err)
	}
	if _, ok := engine.ActiveDeployment("writer"); ok {
		t.Fatal("undeploy left an active route")
	}
	if historical, ok := engine.Deployment(secondDeployment.Ref()); !ok || historical != secondDeployment {
		t.Fatalf("historical replacement = %#v, %v", historical, ok)
	}
	if _, err := engine.Replace(t.Context(), second); !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("Replace after undeploy error = %v, want ErrDeploymentNotFound", err)
	}

	if len(lifecycle) != 3 {
		t.Fatalf("deployment lifecycle events = %d, want deploy + replace + undeploy", len(lifecycle))
	}
	if deployed, ok := lifecycle[0].(event.AgentDeployed); !ok || deployed.Deployment != firstDeployment.Ref() {
		t.Fatalf("first lifecycle event = %#v", lifecycle[0])
	}
	if replaced, ok := lifecycle[1].(event.AgentDeployed); !ok || replaced.Deployment != secondDeployment.Ref() {
		t.Fatalf("second lifecycle event = %#v", lifecycle[1])
	}
	if undeployed, ok := lifecycle[2].(event.AgentUndeployed); !ok || undeployed.Deployment != secondDeployment.Ref() {
		t.Fatalf("third lifecycle event = %#v", lifecycle[2])
	}
}

func TestForgetDeploymentRequiresInactiveUnreferencedDefinition(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	first := deploymentRun("first", 1)
	firstDeployment, err := engine.Deploy(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Run(
		t.Context(),
		first,
		core.Input(deploymentRunInput{Value: 1}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	second := deploymentRun("second", 2)
	secondDeployment, err := engine.Replace(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}

	if err := engine.ForgetDeployment(firstDeployment.Ref()); !errors.Is(err, ErrDeploymentInUse) {
		t.Fatalf("forget referenced deployment error = %v, want ErrDeploymentInUse", err)
	}
	if err := engine.RemoveTree(t.Context(), process.ID()); err != nil {
		t.Fatal(err)
	}
	if err := engine.ForgetDeployment(firstDeployment.Ref()); err != nil {
		t.Fatalf("forget historical deployment: %v", err)
	}
	if _, ok := engine.Deployment(firstDeployment.Ref()); ok {
		t.Fatal("forgotten deployment remains in the catalog")
	}
	if err := engine.ForgetDeployment(secondDeployment.Ref()); !errors.Is(err, ErrDeploymentActive) {
		t.Fatalf("forget active deployment error = %v, want ErrDeploymentActive", err)
	}
	if err := engine.Undeploy(t.Context(), second.Name()); err != nil {
		t.Fatal(err)
	}
	if err := engine.ForgetDeployment(secondDeployment.Ref()); err != nil {
		t.Fatalf("forget undeployed definition: %v", err)
	}
}

func TestAgentRegistrySupportsConcurrentRegistrationAndSnapshots(t *testing.T) {
	const deploymentCount = 32
	deployments := make([]*Deployment, deploymentCount)
	for i := range deploymentCount {
		deployment, err := (deploymentCompiler{}).compile(deploymentFixture(
			fmt.Sprintf("agent-%02d", i),
			core.ConditionSet{"finish": core.True},
			nil,
		))
		if err != nil {
			t.Fatal(err)
		}
		deployments[i] = deployment
	}

	registry := newDeploymentRegistry()
	start := make(chan struct{})
	errs := make(chan error, deploymentCount)
	var wg sync.WaitGroup
	for _, deployment := range deployments {
		wg.Go(func() {
			<-start
			if _, _, err := registry.activate(deployment, false); err != nil {
				errs <- err
			}
		})
	}
	for range 8 {
		wg.Go(func() {
			<-start
			for range deploymentCount {
				listed := registry.listActive()
				for i := 1; i < len(listed); i++ {
					if listed[i-1].agent.Name() > listed[i].agent.Name() {
						errs <- fmt.Errorf("catalog snapshot is not name ordered: %q before %q", listed[i-1].agent.Name(), listed[i].agent.Name())
						return
					}
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	listed := registry.listActive()
	if len(listed) != deploymentCount {
		t.Fatalf("active deployments = %d, want %d", len(listed), deploymentCount)
	}
	for _, deployment := range deployments {
		got, ok := registry.lookup(deployment.Ref())
		if !ok || got != deployment {
			t.Fatalf("ref lookup for %q = %p, %v; want %p", deployment.agent.Name(), got, ok, deployment)
		}
	}
}

type deploymentRunInput struct{ Value int }
type deploymentRunOutput struct{ Value int }

func TestDeployedDefinitionAccessorMutationDoesNotChangeRun(t *testing.T) {
	action := core.NewAction[deploymentRunInput, deploymentRunOutput](
		"double",
		func(_ context.Context, _ *core.ProcessContext, input deploymentRunInput) (deploymentRunOutput, error) {
			return deploymentRunOutput{Value: input.Value * 2}, nil
		},
		core.ActionConfig{},
	)
	source := core.NewAgent(core.AgentConfig{
		Name:        "runner",
		Version:     "1.0.0",
		PlannerName: "goap",
		Actions:     []core.Action{action},
		Goals:       []*core.Goal{core.NewOutputGoal[deploymentRunOutput](core.GoalConfig{Name: "complete"})},
	})
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	if _, err := engine.Deploy(t.Context(), source); err != nil {
		t.Fatal(err)
	}

	// Accessors return collection snapshots. Editing them cannot alter either
	// the source definition or the compiled deployment used by Run.
	actions := source.Actions()
	goals := source.Goals()
	actions[0] = nil
	goals[0] = nil

	process, err := engine.Run(
		t.Context(),
		source,
		core.Input(deploymentRunInput{Value: 21}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("status = %s, want completed", process.Status())
	}
	deployment := existingDeployment(t, engine, source)
	if process.deployment != deployment {
		t.Fatal("process did not retain the catalog deployment handle")
	}
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tree.Snapshots[0]
	if snapshot.Deployment != deployment.Ref() {
		t.Fatalf("snapshot identity drifted with source mutation: %#v", snapshot)
	}
	result, ok := core.Result[deploymentRunOutput](process)
	if !ok || result.Value != 42 {
		t.Fatalf("result = %#v, %v", result, ok)
	}
}

func TestRunCatalogsDefinitionForExactRestore(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	source := deploymentRun("catalog on run", 1)

	process, err := engine.Run(
		t.Context(),
		source,
		core.Input(deploymentRunInput{Value: 20}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	deployment, ok := engine.ActiveDeployment(source.Name())
	if !ok {
		t.Fatal("Run did not install an active deployment")
	}
	if process.deployment != deployment || process.Deployment() != deployment.Ref() {
		t.Fatalf("process deployment = %s, want catalog deployment %s", process.Deployment(), deployment.Ref())
	}
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.RemoveTree(t.Context(), process.ID()); err != nil {
		t.Fatal(err)
	}
	restored, err := engine.RestoreTree(t.Context(), tree, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("restore run-created deployment: %v", err)
	}
	if restored.deployment != deployment {
		t.Fatal("restored process did not resolve the run-created catalog deployment")
	}
}

func TestAdvancedExecutionRejectsForeignDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	foreignEngine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	foreign, err := foreignEngine.Deploy(t.Context(), deploymentRun("foreign", 1))
	if err != nil {
		t.Fatal(err)
	}
	parentDef := deploymentFixture("foreign-parent", core.ConditionSet{"finish": core.True}, nil)
	parent := createProcessForTest(t, engine, parentDef, core.Bindings{}, core.ProcessOptions{})
	ctx := core.WithProcessView(t.Context(), parent)

	tests := []struct {
		name string
		run  func() error
	}{
		{"RunChildWithState", func() error {
			_, err := engine.RunChildWithState(ctx, foreign, nil)
			return err
		}},
		{"RunChild", func() error {
			_, err := engine.RunChild(ctx, foreign, nil)
			return err
		}},
		{"RunDeployment", func() error {
			_, err := engine.RunDeployment(t.Context(), foreign, core.Bindings{}, core.ProcessOptions{})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrDeploymentNotFound) {
				t.Fatalf("error = %v, want ErrDeploymentNotFound", err)
			}
		})
	}
}

func TestChildSpawnBindsCompiledDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	parentDef := deploymentFixture("parent", core.ConditionSet{"finish": core.True}, nil)
	childDef := deploymentFixture("child", core.ConditionSet{"finish": core.True}, nil)
	for _, agentDef := range []*core.Agent{parentDef, childDef} {
		if _, err := engine.Deploy(t.Context(), agentDef); err != nil {
			t.Fatal(err)
		}
	}
	childDeployment := existingDeployment(t, engine, childDef)

	parent := createProcessForTest(t, engine, parentDef, core.Bindings{}, core.ProcessOptions{})
	if started, err := parent.beginRun(); err != nil || !started {
		t.Fatalf("begin parent run = (%v, %v)", started, err)
	}
	defer parent.state.endRun()

	// Child execution takes the immutable deployment handle; accessor snapshots
	// cannot affect execution.
	childActions := childDef.Actions()
	childGoals := childDef.Goals()
	childActions[0] = nil
	childGoals[0] = nil

	child, err := (childRun{
		ctx:        core.WithProcessView(t.Context(), parent),
		engine:     engine,
		deployment: childDeployment,
		mode:       childStartsClean,
	}).admit()
	if err != nil {
		t.Fatal(err)
	}
	if child.deployment != childDeployment {
		t.Fatal("child process did not bind the catalog deployment")
	}
	if got := child.agent().Name(); got != "child" {
		t.Fatalf("child definition name = %q, want frozen name", got)
	}
}

func TestRunBindsCompiledDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	source := deploymentRun("session-deployment", 1)
	if _, err := engine.Deploy(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	deployment := existingDeployment(t, engine, source)

	actions := source.Actions()
	actions[0] = nil
	process, err := engine.Run(
		t.Context(),
		source,
		core.Input(deploymentRunInput{Value: 20}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if process.deployment != deployment {
		t.Fatal("process did not bind the catalog deployment")
	}
}

func TestAgentToolRemainsBoundToConstructionDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	first := deploymentRun("tool-version-one", 1)
	second := deploymentRun("tool-version-two", 2)
	parentDef := deploymentFixture("tool-parent", core.ConditionSet{"finish": core.True}, nil)
	for _, agentDef := range []*core.Agent{first, parentDef} {
		if _, err := engine.Deploy(t.Context(), agentDef); err != nil {
			t.Fatal(err)
		}
	}
	firstDeployment := existingDeployment(t, engine, first)

	tool, err := NewAgentTool[deploymentRunInput, deploymentRunOutput](engine, firstDeployment)
	if err != nil {
		t.Fatal(err)
	}
	boundTool, ok := tool.(*agentTool)
	if !ok {
		t.Fatalf("tool type = %T, want *agentTool", tool)
	}
	if boundTool.deployment != firstDeployment {
		t.Fatal("tool did not bind the active deployment at construction")
	}

	secondDeployment, err := engine.Replace(t.Context(), second)
	if err != nil {
		t.Fatal(err)
	}
	parent := createProcessForTest(t, engine, parentDef, core.Bindings{}, core.ProcessOptions{})
	if started, err := parent.beginRun(); err != nil || !started {
		t.Fatalf("begin parent run = (%v, %v)", started, err)
	}
	defer parent.state.endRun()
	child, err := runChildDeployment(
		core.WithProcessView(t.Context(), parent),
		boundTool.engine,
		boundTool.deployment,
		deploymentRunInput{Value: 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if child.deployment != firstDeployment {
		t.Fatal("pre-redeploy tool drifted to the new active deployment")
	}
	result, ok := core.Result[deploymentRunOutput](child)
	if !ok || result.Value != 21 {
		t.Fatalf("old tool result = %#v, %v; want first deployment", result, ok)
	}

	replacementTool, err := NewAgentTool[deploymentRunInput, deploymentRunOutput](engine, secondDeployment)
	if err != nil {
		t.Fatal(err)
	}
	if replacementTool.(*agentTool).deployment == firstDeployment {
		t.Fatal("tool constructed after redeploy retained the old deployment")
	}
}

func TestAgentToolBindsOneActiveDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	source := deploymentRun("tool-deployment", 3)
	deployment, err := engine.Deploy(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}

	tool, err := NewAgentTool[deploymentRunInput, deploymentRunOutput](engine, deployment)
	if err != nil {
		t.Fatal(err)
	}
	boundTool := tool.(*agentTool)
	boundDeployment := boundTool.deployment

	// Tool construction resolves the active route once. Mutating accessor
	// snapshots cannot change the frozen definition.
	actions := source.Actions()
	goals := source.Goals()
	actions[0] = nil
	goals[0] = nil
	if got := tool.Definition().Name; got != "replaceable" {
		t.Fatalf("tool definition name = %q, want frozen name", got)
	}
	parentDef := deploymentFixture("direct-tool-parent", core.ConditionSet{"finish": core.True}, nil)
	parent := createProcessForTest(t, engine, parentDef, core.Bindings{}, core.ProcessOptions{})
	if started, err := parent.beginRun(); err != nil || !started {
		t.Fatalf("begin parent run = (%v, %v)", started, err)
	}
	defer parent.state.endRun()
	ctx := core.WithProcessView(t.Context(), parent)

	child, err := runChildDeployment(ctx, boundTool.engine, boundTool.deployment, deploymentRunInput{Value: 20})
	if err != nil {
		t.Fatal(err)
	}
	if child.deployment != boundDeployment {
		t.Fatal("synchronous tool drifted from its construction deployment")
	}
	result, ok := core.Result[deploymentRunOutput](child)
	if !ok || result.Value != 23 {
		t.Fatalf("tool result = %#v, %v; want frozen implementation", result, ok)
	}

}

func TestRestoreBindsExactHistoricalDeployment(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	source := deploymentRun("restore-deployment", 1)
	if _, err := engine.Deploy(t.Context(), source); err != nil {
		t.Fatal(err)
	}
	deployment := existingDeployment(t, engine, source)
	replacement := deploymentRun("replacement-deployment", 2)
	if _, err := engine.Replace(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Second)

	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "restored-deployment-process",
		Deployment:    deployment.Ref(),
		StartedAt:     started,
		Status:        core.StatusCompleted,
	}
	restored, err := engine.RestoreTree(t.Context(), core.ProcessSnapshotTree{
		RootID: snapshot.ID, Snapshots: []core.ProcessSnapshot{snapshot},
	}, core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if restored.deployment != deployment {
		t.Fatal("restored process did not bind the exact historical deployment")
	}

	tampered := deployment.Ref()
	tampered.Digest = "different"
	snapshot = core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "mismatched-deployment-process",
		Deployment:    tampered,
		StartedAt:     started,
		Status:        core.StatusCompleted,
	}
	_, err = engine.RestoreTree(t.Context(), core.ProcessSnapshotTree{
		RootID: snapshot.ID, Snapshots: []core.ProcessSnapshot{snapshot},
	}, core.ProcessOptions{})
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("mismatched restore error = %v, want ErrDeploymentNotFound", err)
	}
}

func TestReplaceDoesNotChangeExistingProcessDefinition(t *testing.T) {
	engine := MustNew(Config{Extensions: []core.Extension{goap.NewPlanner()}})
	first := deploymentRun("version-one", 1)
	second := deploymentRun("version-two", 2)
	if _, err := engine.Deploy(t.Context(), first); err != nil {
		t.Fatal(err)
	}

	existing := createProcessForTest(
		t, engine, first, core.Input(deploymentRunInput{Value: 20}), core.ProcessOptions{},
	)
	firstDigest := existingDeployment(t, engine, first).Ref().Digest

	if _, err := engine.Replace(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	active, ok := engine.ActiveDeployment(second.Name())
	if !ok || active.ref.Digest == firstDigest {
		t.Fatalf("active replacement = %#v, %v", active, ok)
	}

	if err := engine.Continue(t.Context(), existing.ID()); err != nil {
		t.Fatal(err)
	}
	firstResult, ok := core.Result[deploymentRunOutput](existing)
	if !ok || firstResult.Value != 21 {
		t.Fatalf("existing process drifted to replacement: %#v, %v", firstResult, ok)
	}

	replacement, err := engine.Run(
		t.Context(),
		second,
		core.Input(deploymentRunInput{Value: 20}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, ok := core.Result[deploymentRunOutput](replacement)
	if !ok || secondResult.Value != 22 {
		t.Fatalf("replacement result = %#v, %v", secondResult, ok)
	}

	_, err = engine.Run(
		t.Context(),
		first,
		core.Input(deploymentRunInput{Value: 20}),
		core.ProcessOptions{},
	)
	if !errors.Is(err, ErrDeploymentConflict) {
		t.Fatalf("run superseded definition error = %v, want ErrDeploymentConflict", err)
	}
}

func existingDeployment(t *testing.T, engine *Engine, source *core.Agent) *Deployment {
	t.Helper()
	deployment, ok := engine.ActiveDeployment(source.Name())
	if !ok {
		t.Fatalf("deployment for %q not found", source.Name())
	}
	return deployment
}

func deploymentRun(description string, increment int) *core.Agent {
	action := core.NewAction[deploymentRunInput, deploymentRunOutput](
		"increment",
		func(_ context.Context, _ *core.ProcessContext, input deploymentRunInput) (deploymentRunOutput, error) {
			return deploymentRunOutput{Value: input.Value + increment}, nil
		},
		core.ActionConfig{Description: description},
	)
	return core.NewAgent(core.AgentConfig{
		Name:        "replaceable",
		Version:     "1.0.0",
		PlannerName: "goap",
		Actions:     []core.Action{action},
		Goals:       []*core.Goal{core.NewOutputGoal[deploymentRunOutput](core.GoalConfig{Name: "complete"})},
	})
}

func deploymentFixture(name string, effects core.ConditionSet, run func(*core.ProcessContext) core.ActionStatus) *core.Agent {
	return deploymentFixtureWith(
		name,
		effects,
		run,
		[]string{"finish"},
		"goap",
	)
}

func deploymentFixtureWith(
	name string,
	effects core.ConditionSet,
	run func(*core.ProcessContext) core.ActionStatus,
	pre []string,
	planner string,
) *core.Agent {
	metadata := deploymentActionMetadata("finish")
	metadata.Effects = effects
	return core.NewAgent(core.AgentConfig{
		Name:        name,
		Description: "fixture",
		Version:     "1.0.0",
		PlannerName: planner,
		Actions:     []core.Action{&mutableDeploymentAction{metadata: metadata, run: run}},
		Goals:       []*core.Goal{core.NewGoal(core.GoalConfig{Name: "complete", Preconditions: pre})},
	})
}

func reconfigureAgent(source *core.Agent, configure func(*core.AgentConfig)) *core.Agent {
	config := core.AgentConfig{
		Name:        source.Name(),
		Description: source.Description(),
		Version:     source.Version(),
		StuckPolicy: source.StuckPolicy(),
		Actions:     source.Actions(),
		Goals:       source.Goals(),
		Conditions:  source.Conditions(),
		PlannerName: source.PlannerName(),
	}
	configure(&config)
	return core.NewAgent(config)
}

func deploymentActionMetadata(effect string) core.ActionMetadata {
	return core.ActionMetadata{
		Name:          "write",
		Inputs:        []core.Binding{{Name: "input", Type: "string"}},
		Outputs:       []core.Binding{{Name: "output", Type: "string"}},
		Preconditions: core.ConditionSet{"ready": core.True},
		Effects:       core.ConditionSet{effect: core.True},
		ToolGroups:    []string{"filesystem"},
		Cost:          core.FixedScore(1),
		Value:         core.FixedScore(2),
	}
}
