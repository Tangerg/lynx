package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEngineStartsSameDeploymentChildWithStableRelation(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "parent"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	rootResult, err := root.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	output := childTestResult(t, rootResult)
	if len(output.ChildIDs) != 1 || output.Failures != 0 {
		t.Fatalf("root output = %#v", output)
	}
	childID, err := ParseProcessID(output.ChildIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	child, found := engine.Process(childID)
	if !found {
		t.Fatal("Engine did not retain the child Process")
	}
	childResult, err := child.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	completed := childResult.Status() == StatusCompleted &&
		childResult.Termination().Cause() == TerminationCauseCompletion
	parentCanceled := childResult.Status() == StatusCanceled &&
		childResult.Termination().Cause() == TerminationCauseParentCancellation
	if !completed && !parentCanceled {
		t.Fatalf("child termination = %#v", childResult.Termination())
	}
	childSnapshot, err := child.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rootRelation := root.Relation()
	childRelation := child.Relation()
	parentID, hasParent := childRelation.ParentID()
	childKey, hasKey := childRelation.ChildKey()
	if !rootRelation.IsRoot() || rootRelation.RootID() != root.ID() || rootRelation.Depth() != 0 {
		t.Fatalf("root relation = %#v", rootRelation)
	}
	if !hasParent || parentID != root.ID() || !hasKey || childKey.String() != "worker" ||
		childRelation.RootID() != root.ID() || childRelation.Depth() != 1 {
		t.Fatalf("child relation = %#v", childRelation)
	}
	if childSnapshot.Relation() != childRelation {
		t.Fatalf("captured child relation = %#v, want %#v", childSnapshot.Relation(), childRelation)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineRejectsDuplicateChildKeyInOneParent(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "duplicate"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	rootResult, err := root.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	output := childTestResult(t, rootResult)
	if len(output.ChildIDs) != 1 || output.Failures != 1 {
		t.Fatalf("duplicate child output = %#v", output)
	}
	childID, _ := ParseProcessID(output.ChildIDs[0])
	child, found := engine.Process(childID)
	if !found {
		t.Fatal("successful child is missing")
	}
	if _, err := child.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineWaitsForChildrenByConditionAndReturnsRequestOrder(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		release  []string
		wantKeys []string
	}{
		{name: "any", mode: "wait:any", release: []string{"third"}, wantKeys: []string{"third"}},
		{name: "quorum", mode: "wait:quorum", release: []string{"third", "first"}, wantKeys: []string{"first", "third"}},
		{name: "all", mode: "wait:all", release: []string{"third", "first", "second"}, wantKeys: []string{"first", "second", "third"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher := newBlockingChildDispatcher("first", "second", "third")
			t.Cleanup(dispatcher.ReleaseAll)
			deployment := newChildTestDeploymentWithDispatcher(t, dispatcher)
			engine, err := NewEngine(EngineConfig{})
			if err != nil {
				t.Fatal(err)
			}
			input, _ := EncodeInput(childTestInput{Mode: test.mode})
			root, err := engine.Start(context.Background(), deployment, input)
			if err != nil {
				t.Fatal(err)
			}
			for range 3 {
				<-dispatcher.started
			}
			waitForProcessStatus(t, root, StatusWaiting)
			waitID, waiting := root.WaitID()
			if !waiting {
				t.Fatal("Waiting parent did not expose its current WaitID")
			}
			externalID, _ := ParseSignalID("signal:forged-child-completion")
			forged, _ := NewSignalRequest(externalID, waitID, json.RawMessage(`{"forged":true}`))
			if accepted, err := root.DeliverSignal(context.Background(), forged); accepted || !errors.Is(err, ErrSignalRejected) {
				t.Fatalf("forged child completion accepted = %t, error = %v", accepted, err)
			}
			for _, name := range test.release {
				dispatcher.Release(name)
			}
			result, err := root.Await(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			output := childTestResult(t, result)
			if !slices.Equal(output.CompletedKeys, test.wantKeys) {
				t.Fatalf("completed keys = %v, want %v", output.CompletedKeys, test.wantKeys)
			}
			dispatcher.ReleaseAll()
			for _, id := range output.ChildIDs {
				childID, _ := ParseProcessID(id)
				child, found := engine.Process(childID)
				if !found {
					t.Fatalf("child %s is missing", childID)
				}
				if _, err := child.Await(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEngineSupportsBoundedSameDefinitionRecursion(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{TreeLimits: TreeLimits{
		MaxDepth: 3, MaxChildren: 2, MaxActiveChildren: 2, MaxTreeProcesses: 4,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "recurse:3"})
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	rootID := process.ID()
	for depth := uint32(0); depth < 3; depth++ {
		result, err := process.Await(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		output := childTestResult(t, result)
		if len(output.ChildIDs) != 1 || output.Failures != 0 {
			t.Fatalf("depth %d output = %#v", depth, output)
		}
		childID, _ := ParseProcessID(output.ChildIDs[0])
		child, found := engine.Process(childID)
		if !found {
			t.Fatalf("depth %d child is missing", depth+1)
		}
		if child.Relation().Depth() != depth+1 || child.Relation().RootID() != rootID {
			t.Fatalf("depth %d relation = %#v", depth+1, child.Relation())
		}
		process = child
	}
	if result, err := process.Await(context.Background()); err != nil || result.Status() != StatusCompleted {
		t.Fatalf("recursive leaf result = %#v, error = %v", result, err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineEnforcesChildDepthFanoutActiveAndTreeLimits(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		deployment := newChildTestDeployment(t)
		engine, err := NewEngine(EngineConfig{TreeLimits: TreeLimits{
			MaxDepth: 1, MaxChildren: 2, MaxActiveChildren: 2, MaxTreeProcesses: 3,
		}})
		if err != nil {
			t.Fatal(err)
		}
		input, _ := EncodeInput(childTestInput{Mode: "recurse:2"})
		root, err := engine.Start(context.Background(), deployment, input)
		if err != nil {
			t.Fatal(err)
		}
		rootOutput := childTestResult(t, mustAwait(t, root))
		childID, _ := ParseProcessID(rootOutput.ChildIDs[0])
		child, _ := engine.Process(childID)
		childOutput := childTestResult(t, mustAwait(t, child))
		if childOutput.Failures != 1 || !slices.Contains(childOutput.FailureCodes, "engine.child.tree_limit") {
			t.Fatalf("depth-limited child output = %#v", childOutput)
		}
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lifetime fanout", func(t *testing.T) {
		deployment := newChildTestDeployment(t)
		engine, err := NewEngine(EngineConfig{TreeLimits: TreeLimits{
			MaxDepth: 2, MaxChildren: 2, MaxActiveChildren: 2, MaxTreeProcesses: 4,
		}})
		if err != nil {
			t.Fatal(err)
		}
		input, _ := EncodeInput(childTestInput{Mode: "fanout"})
		root, err := engine.Start(context.Background(), deployment, input)
		if err != nil {
			t.Fatal(err)
		}
		output := childTestResult(t, mustAwait(t, root))
		if len(output.ChildIDs) != 2 || output.Failures != 1 {
			t.Fatalf("fanout-limited output = %#v", output)
		}
		awaitChildren(t, engine, output.ChildIDs)
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("active children", func(t *testing.T) {
		dispatcher := newBlockingChildDispatcher("first", "second", "third")
		t.Cleanup(dispatcher.ReleaseAll)
		deployment := newChildTestDeploymentWithDispatcher(t, dispatcher)
		engine, err := NewEngine(EngineConfig{TreeLimits: TreeLimits{
			MaxDepth: 2, MaxChildren: 3, MaxActiveChildren: 1, MaxTreeProcesses: 4,
		}})
		if err != nil {
			t.Fatal(err)
		}
		input, _ := EncodeInput(childTestInput{Mode: "fanout_blocking"})
		root, err := engine.Start(context.Background(), deployment, input)
		if err != nil {
			t.Fatal(err)
		}
		<-dispatcher.started
		output := childTestResult(t, mustAwait(t, root))
		if len(output.ChildIDs) != 1 || output.Failures != 2 {
			t.Fatalf("active-child-limited output = %#v", output)
		}
		dispatcher.ReleaseAll()
		awaitChildren(t, engine, output.ChildIDs)
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("tree processes", func(t *testing.T) {
		deployment := newChildTestDeployment(t)
		engine, err := NewEngine(EngineConfig{TreeLimits: TreeLimits{
			MaxDepth: 2, MaxChildren: 3, MaxActiveChildren: 3, MaxTreeProcesses: 2,
		}})
		if err != nil {
			t.Fatal(err)
		}
		input, _ := EncodeInput(childTestInput{Mode: "fanout"})
		root, err := engine.Start(context.Background(), deployment, input)
		if err != nil {
			t.Fatal(err)
		}
		output := childTestResult(t, mustAwait(t, root))
		if len(output.ChildIDs) != 1 || output.Failures != 2 {
			t.Fatalf("tree-process-limited output = %#v", output)
		}
		awaitChildren(t, engine, output.ChildIDs)
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTreeProcessLimitBoundsRecursiveBinaryExpansion(t *testing.T) {
	deployment := newChildTestDeployment(t)
	limits := DefaultLimits()
	limits.MaxSteps = 100_000
	limits.MaxEffects = 100_000
	limits.MaxSignals = 100_000
	limits.MaxPendingSignals = 100_000
	engine, err := NewEngine(EngineConfig{
		Limits: limits,
		TreeLimits: TreeLimits{
			MaxDepth: 8, MaxChildren: 2, MaxActiveChildren: 2, MaxTreeProcesses: 15,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "binary:8"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result := mustAwait(t, root); result.Status() != StatusCompleted {
		t.Fatalf("root status = %s", result.Status())
	}
	engine.mu.RLock()
	processCount := 0
	var deepest uint32
	for _, controller := range engine.processes {
		if controller.relation.RootID() == root.ID() {
			processCount++
			deepest = max(deepest, controller.relation.Depth())
		}
	}
	engine.mu.RUnlock()
	if processCount != 15 || deepest > 8 {
		t.Fatalf("bounded tree Process count = %d, deepest = %d", processCount, deepest)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineAttenuatesChildBudgetAndCapabilities(t *testing.T) {
	read, _ := ParseCapability("resource.read")
	rootCapabilities, _ := NewCapabilitySet(read)
	deployment := newChildTestDeployment(t)

	t.Run("subset", func(t *testing.T) {
		engine, err := NewEngine(EngineConfig{Capabilities: rootCapabilities})
		if err != nil {
			t.Fatal(err)
		}
		input, _ := EncodeInput(childTestInput{Mode: "capability_child"})
		root, err := engine.Start(context.Background(), deployment, input)
		if err != nil {
			t.Fatal(err)
		}
		output := childTestResult(t, mustAwait(t, root))
		childID, _ := ParseProcessID(output.ChildIDs[0])
		child, _ := engine.Process(childID)
		if !child.Capabilities().Contains(read) || child.Budget() != (Budget{Steps: 20, Effects: 20, Signals: 40}) {
			t.Fatalf("child capabilities = %#v, budget = %#v", child.Capabilities(), child.Budget())
		}
		_ = mustAwait(t, child)
		if err := engine.Close(); err != nil {
			t.Fatal(err)
		}
	})

	for _, test := range []struct {
		name string
		mode string
		code string
	}{
		{name: "capability escalation", mode: "capability_escalation", code: "engine.child.capability_escalation"},
		{name: "budget escalation", mode: "budget_escalation", code: "engine.child.budget_exhausted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine, err := NewEngine(EngineConfig{Capabilities: rootCapabilities})
			if err != nil {
				t.Fatal(err)
			}
			input, _ := EncodeInput(childTestInput{Mode: test.mode})
			root, err := engine.Start(context.Background(), deployment, input)
			if err != nil {
				t.Fatal(err)
			}
			output := childTestResult(t, mustAwait(t, root))
			if output.Failures != 1 || !slices.Contains(output.FailureCodes, test.code) {
				t.Fatalf("attenuation output = %#v", output)
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParentTerminationPropagatesAtChildSafeBoundary(t *testing.T) {
	tests := []struct {
		name       string
		terminate  func(t *testing.T, process *Process)
		wantParent Status
		wantChild  Status
		wantCause  TerminationCause
	}{
		{
			name: "completion", wantParent: StatusCompleted,
			wantChild: StatusCanceled, wantCause: TerminationCauseParentCancellation,
		},
		{
			name: "kill", wantParent: StatusKilled,
			wantChild: StatusCanceled, wantCause: TerminationCauseParentCancellation,
			terminate: func(t *testing.T, process *Process) {
				t.Helper()
				if err := process.Kill(context.Background(), "stop parent tree"); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatcher := newBlockingChildDispatcher("first", "second", "third")
			t.Cleanup(dispatcher.ReleaseAll)
			deployment := newChildTestDeploymentWithDispatcher(t, dispatcher)
			mode := "fanout_blocking"
			if test.terminate != nil {
				mode = "wait:all"
			}
			engine, err := NewEngine(EngineConfig{})
			if err != nil {
				t.Fatal(err)
			}
			input, _ := EncodeInput(childTestInput{Mode: mode})
			parent, err := engine.Start(context.Background(), deployment, input)
			if err != nil {
				t.Fatal(err)
			}
			if test.terminate != nil {
				for range 3 {
					<-dispatcher.started
				}
				waitForProcessStatus(t, parent, StatusWaiting)
				test.terminate(t, parent)
			}
			parentResult := mustAwait(t, parent)
			if parentResult.Status() != test.wantParent {
				t.Fatalf("parent status = %s, want %s", parentResult.Status(), test.wantParent)
			}
			var childIDs []string
			if parentResult.Status() == StatusCompleted {
				childIDs = childTestResult(t, parentResult).ChildIDs
			} else {
				childIDs = directChildIDs(t, engine, parent.ID())
			}
			dispatcher.ReleaseAll()
			for _, encoded := range childIDs {
				childID, _ := ParseProcessID(encoded)
				child, _ := engine.Process(childID)
				result := mustAwait(t, child)
				if result.Status() != test.wantChild || result.Termination().Cause() != test.wantCause {
					t.Fatalf("child result = status %s cause %s", result.Status(), result.Termination().Cause())
				}
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParentDeadlinePropagatesAsParentDeadline(t *testing.T) {
	dispatcher := newBlockingChildDispatcher("first", "second", "third")
	t.Cleanup(dispatcher.ReleaseAll)
	deployment := newChildTestDeploymentWithDispatcher(t, dispatcher)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	input, _ := EncodeInput(childTestInput{Mode: "wait:all"})
	parent, err := engine.Start(ctx, deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		<-dispatcher.started
	}
	parentResult := mustAwait(t, parent)
	if parentResult.Status() != StatusTimedOut || parentResult.Termination().Cause() != TerminationCauseHostDeadline {
		t.Fatalf("parent termination = %#v", parentResult.Termination())
	}
	childIDs := directChildIDs(t, engine, parent.ID())
	dispatcher.ReleaseAll()
	for _, encoded := range childIDs {
		childID, _ := ParseProcessID(encoded)
		child, _ := engine.Process(childID)
		result := mustAwait(t, child)
		if result.Status() != StatusTimedOut || result.Termination().Cause() != TerminationCauseParentDeadline {
			t.Fatalf("child termination = %#v", result.Termination())
		}
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestChildFailureRemainsExplicitStrategyInput(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "wait:failure"})
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != StatusFailed {
		t.Fatalf("parent status = %s", result.Status())
	}
	failure, ok := result.Termination().Failure()
	if !ok || failure.Code() != "test.child.failed" {
		t.Fatalf("parent failure = %#v", failure)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEngineRejectsWaitingOnDescendantThatIsNotDirectChild(t *testing.T) {
	deployment := newChildTestDeployment(t)
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := EncodeInput(childTestInput{Mode: "recurse:2"})
	root, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	rootOutput := childTestResult(t, mustAwait(t, root))
	childID, _ := ParseProcessID(rootOutput.ChildIDs[0])
	child, _ := engine.Process(childID)
	childOutput := childTestResult(t, mustAwait(t, child))
	grandchildID, _ := ParseProcessID(childOutput.ChildIDs[0])
	waitID, _ := ParseWaitID("wait:ancestor-rejected")
	waitKey, _ := ParseWaitKey("descendant")
	_, err = engine.registerChildWait(root.ID(), waitID, ChildWaitSpec{
		Key: waitKey, Children: []ProcessID{grandchildID}, Condition: AllChildren(),
	})
	if !errors.Is(err, ErrInvalidChildWait) {
		t.Fatalf("ancestor wait error = %v, want %v", err, ErrInvalidChildWait)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

type childTestInput struct {
	Mode string `json:"mode"`
}

type childTestOutput struct {
	ChildIDs      []string `json:"child_ids,omitempty"`
	CompletedKeys []string `json:"completed_keys,omitempty"`
	FailureCodes  []string `json:"failure_codes,omitempty"`
	Failures      int      `json:"failures"`
}

type childTestState struct {
	Phase    string   `json:"phase"`
	Mode     string   `json:"mode"`
	ChildIDs []string `json:"child_ids,omitempty"`
	WaitID   string   `json:"wait_id,omitempty"`
}

type childTestDefinition struct {
	descriptor Descriptor
	reference  DeploymentRef
}

func newChildTestDeployment(t *testing.T) Deployment {
	return newChildTestDeploymentWithDispatcher(t, childTestDispatcher{})
}

func newChildTestDeploymentWithDispatcher(t *testing.T, dispatcher Dispatcher) Deployment {
	t.Helper()
	inputSchema, err := SchemaFor[childTestInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[childTestOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name: "test.child", Description: "Exercise child Process framework Effects.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	implementation := ComputeDigest([]byte("child-test-implementation"))
	configuration := ComputeDigest([]byte("child-test-configuration"))
	reference, err := NewDeploymentRef(descriptor, implementation, configuration)
	if err != nil {
		t.Fatal(err)
	}
	definition := &childTestDefinition{descriptor: descriptor, reference: reference}
	deployment, err := NewDeployment(DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: implementation, ConfigurationDigest: configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func (definition *childTestDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *childTestDefinition) Start(input Input) (Execution, error) {
	decoded, err := DecodeInput[childTestInput](input)
	if err != nil {
		return nil, err
	}
	return &childTestExecution{
		reference: definition.reference,
		state:     childTestState{Phase: "ready", Mode: decoded.Mode},
	}, nil
}

func (definition *childTestDefinition) Restore(state ExecutionState) (Execution, error) {
	if state.Kind() != "test.child" || state.SchemaVersion() != 1 {
		return nil, ErrInvalidExecutionState
	}
	var decoded childTestState
	if err := json.Unmarshal(state.Payload(), &decoded); err != nil {
		return nil, err
	}
	return &childTestExecution{reference: definition.reference, state: decoded}, nil
}

type childTestExecution struct {
	reference DeploymentRef
	state     childTestState
}

func (execution *childTestExecution) Step(_ context.Context, signals []Signal) (Transition, error) {
	switch execution.state.Phase {
	case "ready":
		if execution.state.Mode == "leaf" || execution.state.Mode == "recurse:0" ||
			execution.state.Mode == "binary:0" {
			execution.state.Phase = "done"
			output, _ := EncodeOutput(childTestOutput{})
			return Complete(0, output)
		}
		if execution.state.Mode == "leaf_pause" {
			execution.state.Phase = "leaf_paused"
			return Pause(0, "child paused for tree capture")
		}
		if execution.state.Mode == "leaf_fail" {
			execution.state.Phase = "done"
			failure, _ := NewFailure(FailureKindExecution, "test.leaf.failed", "leaf failed as requested")
			return Fail(0, failure)
		}
		if execution.state.Mode == "external_wait" {
			execution.state.Phase = "external_wait_opened"
			key, _ := ParseWaitKey("external_input")
			effect, err := RequestWait(key, json.RawMessage(`{"kind":"external_input"}`))
			if err != nil {
				return Transition{}, err
			}
			return Continue(0, effect)
		}
		if strings.HasPrefix(execution.state.Mode, "leaf:") {
			payload, _ := json.Marshal(struct {
				Name string `json:"name"`
			}{Name: strings.TrimPrefix(execution.state.Mode, "leaf:")})
			effect, err := NewDispatcherEffect(payload)
			if err != nil {
				return Transition{}, err
			}
			execution.state.Phase = "leaf_effect"
			return Continue(0, effect)
		}
		execution.state.Phase = "started"
		if strings.HasPrefix(execution.state.Mode, "wait:") {
			var effects []Effect
			names := []string{"first", "second", "third"}
			switch execution.state.Mode {
			case "wait:subtree":
				names = []string{"target"}
			case "wait:subtree_all":
				names = []string{"target", "sibling"}
			}
			for _, name := range names {
				mode := "leaf:" + name
				if execution.state.Mode == "wait:subtree_all" && name == "sibling" {
					mode = "leaf_pause"
				}
				if execution.state.Mode == "wait:paused" {
					mode = "leaf_pause"
				}
				if execution.state.Mode == "wait:failure" {
					mode = "leaf"
					if name == "first" {
						mode = "leaf_fail"
					}
				}
				if (execution.state.Mode == "wait:subtree" || execution.state.Mode == "wait:subtree_all") &&
					name == "target" {
					mode = "nested_wait"
				}
				childInput, _ := EncodeInput(childTestInput{Mode: mode})
				key, _ := ParseChildKey(name)
				effect, err := StartChild(childTestSpec(key, execution.reference, childInput))
				if err != nil {
					return Transition{}, err
				}
				effects = append(effects, effect)
			}
			return Continue(0, effects...)
		}
		if strings.HasPrefix(execution.state.Mode, "binary:") {
			depth, err := strconv.Atoi(strings.TrimPrefix(execution.state.Mode, "binary:"))
			if err != nil || depth <= 0 || depth > 16 {
				return Transition{}, errors.New("invalid binary child depth")
			}
			var effects []Effect
			for _, name := range []string{"left", "right"} {
				childInput, _ := EncodeInput(childTestInput{Mode: fmt.Sprintf("binary:%d", depth-1)})
				key, _ := ParseChildKey(name)
				spec := childTestSpec(key, execution.reference, childInput)
				units := uint64(20*(1<<uint(depth-1)) - 10)
				spec.Budget, _ = NewBudget(units, units, units)
				effect, err := StartChild(spec)
				if err != nil {
					return Transition{}, err
				}
				effects = append(effects, effect)
			}
			return Continue(0, effects...)
		}
		if execution.state.Mode == "fanout" || execution.state.Mode == "fanout_blocking" {
			var effects []Effect
			for _, name := range []string{"first", "second", "third"} {
				mode := "leaf"
				if execution.state.Mode == "fanout_blocking" {
					mode = "leaf:" + name
				}
				childInput, _ := EncodeInput(childTestInput{Mode: mode})
				key, _ := ParseChildKey(name)
				effect, err := StartChild(childTestSpec(key, execution.reference, childInput))
				if err != nil {
					return Transition{}, err
				}
				effects = append(effects, effect)
			}
			return Continue(0, effects...)
		}
		childMode := "leaf"
		recursiveDepth := 0
		if strings.HasPrefix(execution.state.Mode, "recurse:") {
			depth, err := strconv.Atoi(strings.TrimPrefix(execution.state.Mode, "recurse:"))
			if err != nil || depth <= 0 {
				return Transition{}, errors.New("invalid recursive child depth")
			}
			recursiveDepth = depth
			childMode = fmt.Sprintf("recurse:%d", depth-1)
		}
		if execution.state.Mode == "nested_wait" {
			childMode = "external_wait"
		}
		childInput, _ := EncodeInput(childTestInput{Mode: childMode})
		key, _ := ParseChildKey("worker")
		spec := childTestSpec(key, execution.reference, childInput)
		if execution.state.Mode == "nested_wait" {
			spec.Budget, _ = NewBudget(5, 5, 5)
		}
		if recursiveDepth > 0 {
			units := uint64(recursiveDepth * 50)
			spec.Budget, _ = NewBudget(units, units, units)
		}
		switch execution.state.Mode {
		case "capability_child":
			capability, _ := ParseCapability("resource.read")
			spec.Capabilities, _ = NewCapabilitySet(capability)
		case "capability_escalation":
			capability, _ := ParseCapability("resource.write")
			spec.Capabilities, _ = NewCapabilitySet(capability)
		case "budget_escalation":
			spec.Budget, _ = NewBudget(20_000, 20_000, 20_000)
		}
		effect, err := StartChild(spec)
		if err != nil {
			return Transition{}, err
		}
		if execution.state.Mode == "duplicate" {
			return Continue(0, effect, effect)
		}
		return Continue(0, effect)
	case "started":
		if len(signals) == 0 {
			return Transition{}, errors.New("child start results are required")
		}
		output := childTestOutput{}
		for _, signal := range signals {
			result, err := ParseChildStartResult(signal)
			if err != nil {
				return Transition{}, err
			}
			if childID, started := result.ProcessID(); started {
				output.ChildIDs = append(output.ChildIDs, childID.String())
			} else if failure, failed := result.Failure(); failed {
				output.Failures++
				output.FailureCodes = append(output.FailureCodes, failure.Code())
			}
		}
		needsChildWait := len(output.ChildIDs) > 0 && (strings.HasPrefix(execution.state.Mode, "wait:") ||
			strings.HasPrefix(execution.state.Mode, "recurse:") ||
			strings.HasPrefix(execution.state.Mode, "binary:") || execution.state.Mode == "nested_wait")
		if !needsChildWait {
			execution.state.Phase = "done"
			erased, _ := EncodeOutput(output)
			return Complete(uint32(len(signals)), erased)
		}
		execution.state.ChildIDs = slices.Clone(output.ChildIDs)
		children := make([]ProcessID, len(output.ChildIDs))
		for index, encoded := range output.ChildIDs {
			children[index], _ = ParseProcessID(encoded)
		}
		condition := AllChildren()
		switch execution.state.Mode {
		case "wait:any":
			condition = AnyChild()
		case "wait:quorum":
			condition, _ = ChildQuorum(2)
		}
		key, _ := ParseWaitKey("children")
		effect, err := WaitForChildren(ChildWaitSpec{Key: key, Children: children, Condition: condition})
		if err != nil {
			return Transition{}, err
		}
		execution.state.Phase = "wait_opened"
		return Continue(uint32(len(signals)), effect)
	case "wait_opened":
		if len(signals) == 0 {
			return Transition{}, errors.New("child wait-opened Signal is required")
		}
		opened, err := ParseChildWaitOpened(signals[0])
		if err != nil {
			return Transition{}, err
		}
		openedSpec := opened.Spec()
		if len(openedSpec.Children) != len(execution.state.ChildIDs) {
			return Transition{}, errors.New("child wait acknowledgement changed the requested children")
		}
		if len(openedSpec.Children) > 0 {
			original := openedSpec.Children[0]
			openedSpec.Children[0] = ProcessID{}
			if opened.Spec().Children[0] != original {
				return Transition{}, errors.New("child wait acknowledgement exposed mutable children")
			}
		}
		execution.state.WaitID = opened.WaitID().String()
		if len(signals) > 1 {
			return execution.completeChildren(signals, uint32(len(signals)))
		}
		execution.state.Phase = "waiting"
		return Wait(1, opened.WaitID())
	case "waiting":
		return execution.completeChildren(signals, uint32(len(signals)))
	case "external_wait_opened":
		if len(signals) != 1 {
			return Transition{}, errors.New("external wait-opened Signal is required")
		}
		waitID, addressed := signals[0].WaitID()
		if !addressed {
			return Transition{}, errors.New("external wait-opened Signal has no WaitID")
		}
		execution.state.WaitID = waitID.String()
		execution.state.Phase = "external_waiting"
		return Wait(1, waitID)
	case "external_waiting":
		if len(signals) != 1 {
			return Transition{}, errors.New("external wait response is required")
		}
		execution.state.Phase = "done"
		output, _ := EncodeOutput(childTestOutput{})
		return Complete(1, output)
	case "leaf_effect":
		if len(signals) != 1 {
			return Transition{}, errors.New("leaf Effect settlement is required")
		}
		execution.state.Phase = "done"
		output, _ := EncodeOutput(childTestOutput{})
		return Complete(1, output)
	case "leaf_paused":
		execution.state.Phase = "done"
		output, _ := EncodeOutput(childTestOutput{})
		return Complete(0, output)
	default:
		return Transition{}, errors.New("child test execution cannot advance")
	}
}

func (execution *childTestExecution) completeChildren(signals []Signal, consumedSignals uint32) (Transition, error) {
	if len(signals) == 0 {
		return Transition{}, errors.New("children-completed Signal is required")
	}
	completed, err := ParseChildrenCompleted(signals[len(signals)-1])
	if err != nil {
		return Transition{}, err
	}
	if completed.WaitID().String() != execution.state.WaitID {
		return Transition{}, errors.New("children completed another wait")
	}
	output := childTestOutput{ChildIDs: slices.Clone(execution.state.ChildIDs)}
	for _, outcome := range completed.Outcomes() {
		if outcome.Result().Status() == StatusFailed {
			execution.state.Phase = "done"
			failure, _ := NewFailure(FailureKindExecution, "test.child.failed", "a child Process failed")
			return Fail(consumedSignals, failure)
		}
		output.CompletedKeys = append(output.CompletedKeys, outcome.Key().String())
	}
	execution.state.Phase = "done"
	erased, _ := EncodeOutput(output)
	return Complete(consumedSignals, erased)
}

func childTestSpec(key ChildKey, deployment DeploymentRef, input Input) ChildSpec {
	budget, _ := NewBudget(20, 20, 40)
	return ChildSpec{
		Key: key, DeploymentRef: deployment, Input: input, Budget: budget,
		Capabilities: CapabilitySet{},
	}
}

func (execution *childTestExecution) Snapshot() (ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return ExecutionState{}, err
	}
	return NewExecutionState("test.child", 1, payload)
}

type childTestDispatcher struct{}

func (childTestDispatcher) Dispatch(context.Context, EffectRequest, DeltaEmitter) (Settlement, error) {
	return Settlement{}, errors.New("child test has no dispatcher Effects")
}

func (childTestDispatcher) ReplayPolicy(Effect) ReplayPolicy { return ReplayPolicyNever }

type blockingChildDispatcher struct {
	started  chan string
	releases map[string]*childTestRelease
}

func newBlockingChildDispatcher(names ...string) *blockingChildDispatcher {
	dispatcher := &blockingChildDispatcher{
		started: make(chan string, len(names)), releases: make(map[string]*childTestRelease, len(names)),
	}
	for _, name := range names {
		dispatcher.releases[name] = &childTestRelease{done: make(chan struct{})}
	}
	return dispatcher
}

func (dispatcher *blockingChildDispatcher) Dispatch(
	_ context.Context,
	request EffectRequest,
	_ DeltaEmitter,
) (Settlement, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(request.Effect().Payload(), &input); err != nil {
		return Settlement{}, err
	}
	release, exists := dispatcher.releases[input.Name]
	if !exists {
		return Settlement{}, fmt.Errorf("unknown child %q", input.Name)
	}
	dispatcher.started <- input.Name
	<-release.done
	return NewSettlement(request.ID(), SettlementStatusSucceeded, json.RawMessage(`{}`))
}

func (*blockingChildDispatcher) ReplayPolicy(Effect) ReplayPolicy { return ReplayPolicyNever }

func (dispatcher *blockingChildDispatcher) Release(name string) {
	if release := dispatcher.releases[name]; release != nil {
		release.once.Do(func() { close(release.done) })
	}
}

func (dispatcher *blockingChildDispatcher) ReleaseAll() {
	for name := range dispatcher.releases {
		dispatcher.Release(name)
	}
}

type childTestRelease struct {
	done chan struct{}
	once sync.Once
}

func waitForProcessStatus(t *testing.T, process *Process, want Status) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for {
		snapshot, err := process.Capture(ctx)
		if err != nil {
			t.Fatalf("capture Process while waiting for %s: %v", want, err)
		}
		if snapshot.Status() == want {
			return
		}
		runtime.Gosched()
	}
}

func childTestResult(t *testing.T, result Result) childTestOutput {
	t.Helper()
	if result.Status() != StatusCompleted {
		t.Fatalf("status = %s, termination = %#v", result.Status(), result.Termination())
	}
	erased, ok := result.Output()
	if !ok {
		t.Fatal("completed result has no Output")
	}
	decoded, err := DecodeOutput[childTestOutput](erased)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func mustAwait(t *testing.T, process *Process) Result {
	t.Helper()
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func awaitChildren(t *testing.T, engine *Engine, encodedIDs []string) {
	t.Helper()
	for _, encoded := range encodedIDs {
		childID, err := ParseProcessID(encoded)
		if err != nil {
			t.Fatal(err)
		}
		child, found := engine.Process(childID)
		if !found {
			t.Fatalf("child %s is missing", childID)
		}
		_ = mustAwait(t, child)
	}
}

func directChildIDs(t *testing.T, engine *Engine, parentID ProcessID) []string {
	t.Helper()
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	var ids []string
	for _, controller := range engine.processes {
		actualParent, child := controller.relation.ParentID()
		if child && actualParent == parentID {
			ids = append(ids, controller.processID.String())
		}
	}
	slices.Sort(ids)
	return ids
}
