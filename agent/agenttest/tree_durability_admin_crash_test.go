package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

type administrativeCommitGate struct {
	store       *MemoryTreeDurability
	source      agent.Digest
	prospective agent.TreeSnapshot

	reached  chan struct{}
	decision chan error
	resolve  sync.Once
}

func newAdministrativeCommitGate(
	t *testing.T,
	store *MemoryTreeDurability,
	source agent.Digest,
	prospective agent.TreeSnapshot,
) *administrativeCommitGate {
	t.Helper()
	if store == nil || !source.Valid() || !prospective.Valid() ||
		source == prospective.Digest() {
		t.Fatal("invalid administrative crash gate")
	}
	gate := &administrativeCommitGate{
		store: store, source: source, prospective: prospective,
		reached: make(chan struct{}), decision: make(chan error, 1),
	}
	t.Cleanup(func() { gate.abort() })
	return gate
}

func (g *administrativeCommitGate) commit() error {
	g.store.mu.Lock()
	head, exists := g.store.heads[g.prospective.RootID()]
	incarnationID, durable := g.prospective.IncarnationID()
	if !exists || !durable || head.incarnationID != incarnationID ||
		head.digest != g.source {
		g.store.mu.Unlock()
		return treeIncarnationConflict()
	}
	g.store.heads[g.prospective.RootID()] = memoryHead(g.prospective)
	g.store.mu.Unlock()
	close(g.reached)
	return <-g.decision
}

func (g *administrativeCommitGate) await(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case <-g.reached:
	case <-ctx.Done():
		t.Fatalf("administrative commit gate was not reached: %v", ctx.Err())
	}
}

func (g *administrativeCommitGate) abort() {
	g.resolve.Do(func() { g.decision <- errSimulatedHostCrash })
}

type crashTreeRole string

const (
	crashTreeRoleInvalid crashTreeRole = ""
	crashTreeRoleRoot    crashTreeRole = "root"
	crashTreeRoleChild   crashTreeRole = "child"
)

func (r crashTreeRole) valid() bool {
	return r == crashTreeRoleRoot || r == crashTreeRoleChild
}

type crashTreePhase string

const (
	crashTreePhaseInvalid          crashTreePhase = ""
	crashTreePhaseReady            crashTreePhase = "ready"
	crashTreePhaseChildStarting    crashTreePhase = "child_starting"
	crashTreePhaseRootWaitOpening  crashTreePhase = "root_wait_opening"
	crashTreePhaseRootWaiting      crashTreePhase = "root_waiting"
	crashTreePhaseChildWaitOpening crashTreePhase = "child_wait_opening"
	crashTreePhaseChildWaiting     crashTreePhase = "child_waiting"
	crashTreePhaseFinished         crashTreePhase = "finished"
)

func (p crashTreePhase) valid() bool {
	switch p {
	case crashTreePhaseReady, crashTreePhaseChildStarting,
		crashTreePhaseRootWaitOpening, crashTreePhaseRootWaiting,
		crashTreePhaseChildWaitOpening, crashTreePhaseChildWaiting,
		crashTreePhaseFinished:
		return true
	default:
		return false
	}
}

const (
	crashTreeDeploymentName        = "agenttest.durability_crash_tree"
	crashTreeDeploymentDescription = "Creates a waiting child tree for administrative crash recovery."
	crashTreeImplementationSeed    = "agenttest durability crash tree implementation"
	crashTreeConfigurationSeed     = "agenttest durability crash tree configuration"
	crashTreeChildKey              = "worker"
	crashTreeRootWaitKey           = "child_completion"
	crashTreeChildWaitKey          = "external_input"
	crashTreeCancellationReason    = "cancel waiting child after product decision"
	crashTreeWaitPayloadKind       = "external_input"
	crashTreeChildStepBudget       = 2
	crashTreeChildEffectBudget     = 1
	crashTreeChildSignalBudget     = 1
	crashTreeDirectChildDepth      = 1
)

type crashTreeInput struct {
	Role crashTreeRole `json:"role"`
}

type crashTreeOutput struct {
	Completed bool `json:"completed"`
}

type crashTreeState struct {
	Role    crashTreeRole  `json:"role"`
	Phase   crashTreePhase `json:"phase"`
	ChildID string         `json:"child_id,omitempty"`
	WaitID  string         `json:"wait_id,omitempty"`
}

func (s crashTreeState) valid() bool {
	if !s.Role.valid() || !s.Phase.valid() {
		return false
	}
	if s.Role == crashTreeRoleRoot {
		switch s.Phase {
		case crashTreePhaseReady, crashTreePhaseChildStarting:
			return s.ChildID == "" && s.WaitID == ""
		case crashTreePhaseRootWaitOpening:
			_, err := agent.ParseProcessID(s.ChildID)
			return err == nil && s.WaitID == ""
		case crashTreePhaseRootWaiting:
			_, childErr := agent.ParseProcessID(s.ChildID)
			_, waitErr := agent.ParseWaitID(s.WaitID)
			return childErr == nil && waitErr == nil
		case crashTreePhaseFinished:
			return true
		default:
			return false
		}
	}
	switch s.Phase {
	case crashTreePhaseReady, crashTreePhaseChildWaitOpening:
		return s.ChildID == "" && s.WaitID == ""
	case crashTreePhaseChildWaiting:
		_, err := agent.ParseWaitID(s.WaitID)
		return err == nil && s.ChildID == ""
	case crashTreePhaseFinished:
		return true
	default:
		return false
	}
}

type crashTreeDefinition struct {
	descriptor agent.Descriptor
	reference  agent.DeploymentRef
}

func (d *crashTreeDefinition) Descriptor() agent.Descriptor { return d.descriptor }

func (d *crashTreeDefinition) Start(input agent.Input) (agent.Execution, error) {
	decoded, err := input.Decode[crashTreeInput]()
	if err != nil {
		return nil, err
	}
	state := crashTreeState{Role: decoded.Role, Phase: crashTreePhaseReady}
	if !state.valid() {
		return nil, agent.ErrInvalidInput
	}
	return &crashTreeExecution{definition: d, state: state}, nil
}

func (d *crashTreeDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != d.descriptor.Name() {
		return nil, agent.ErrInvalidExecutionState
	}
	var decoded crashTreeState
	if err := json.Unmarshal(state.Payload(), &decoded); err != nil {
		return nil, err
	}
	if !decoded.valid() {
		return nil, agent.ErrInvalidExecutionState
	}
	return &crashTreeExecution{definition: d, state: decoded}, nil
}

type crashTreeExecution struct {
	definition *crashTreeDefinition
	state      crashTreeState
}

func (e *crashTreeExecution) Step(
	_ context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	if e.state.Role == crashTreeRoleRoot {
		return e.stepRoot(signals)
	}
	return e.stepChild(signals)
}

func (e *crashTreeExecution) stepRoot(signals []agent.Signal) (agent.Transition, error) {
	switch e.state.Phase {
	case crashTreePhaseReady:
		return e.startRootChild(signals)
	case crashTreePhaseChildStarting:
		return e.openRootChildWait(signals)
	case crashTreePhaseRootWaitOpening:
		return e.enterRootChildWait(signals)
	case crashTreePhaseRootWaiting:
		return e.completeRoot(signals)
	default:
		return agent.Transition{}, errors.New("agenttest: root cannot advance from its current phase")
	}
}

func (e *crashTreeExecution) startRootChild(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 0 {
		return agent.Transition{}, errors.New("agenttest: root received an unexpected initial Signal")
	}
	input, err := e.definition.descriptor.EncodeInput(crashTreeInput{Role: crashTreeRoleChild})
	if err != nil {
		return agent.Transition{}, err
	}
	key, err := agent.ParseChildKey(crashTreeChildKey)
	if err != nil {
		return agent.Transition{}, err
	}
	budget, err := agent.NewBudget(crashTreeChildStepBudget, crashTreeChildEffectBudget, crashTreeChildSignalBudget)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.StartChild(agent.ChildSpec{
		Key: key, DeploymentRef: e.definition.reference, Input: input,
		Budget: budget, Capabilities: agent.CapabilitySet{},
	})
	if err != nil {
		return agent.Transition{}, err
	}
	e.state.Phase = crashTreePhaseChildStarting
	return agent.Continue(0, effect)
}

func (e *crashTreeExecution) openRootChildWait(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 {
		return agent.Transition{}, errors.New("agenttest: child start result is missing")
	}
	started, err := agent.ParseChildStartResult(signals[0])
	if err != nil {
		return agent.Transition{}, err
	}
	childID, ok := started.ProcessID()
	if !ok {
		return agent.Transition{}, errors.New("agenttest: child did not start")
	}
	key, err := agent.ParseWaitKey(crashTreeRootWaitKey)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.WaitForChildren(agent.ChildWaitSpec{
		Key: key, Children: []agent.ProcessID{childID}, Condition: agent.AllChildren(),
	})
	if err != nil {
		return agent.Transition{}, err
	}
	e.state.ChildID = childID.String()
	e.state.Phase = crashTreePhaseRootWaitOpening
	return agent.Continue(1, effect)
}

func (e *crashTreeExecution) enterRootChildWait(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 {
		return agent.Transition{}, errors.New("agenttest: child wait acknowledgement is missing")
	}
	opened, err := agent.ParseChildWaitOpened(signals[0])
	if err != nil {
		return agent.Transition{}, err
	}
	e.state.WaitID = opened.WaitID().String()
	e.state.Phase = crashTreePhaseRootWaiting
	return agent.Wait(1, opened.WaitID())
}

func (e *crashTreeExecution) completeRoot(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) != 1 {
		return agent.Transition{}, errors.New("agenttest: child completion is missing")
	}
	if _, err := agent.ParseChildrenCompleted(signals[0]); err != nil {
		return agent.Transition{}, err
	}
	e.state.Phase = crashTreePhaseFinished
	output, err := agent.EncodeOutput(crashTreeOutput{Completed: true})
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Complete(1, output)
}

func (e *crashTreeExecution) stepChild(signals []agent.Signal) (agent.Transition, error) {
	switch e.state.Phase {
	case crashTreePhaseReady:
		if len(signals) != 0 {
			return agent.Transition{}, errors.New("agenttest: child received an unexpected initial Signal")
		}
		key, err := agent.ParseWaitKey(crashTreeChildWaitKey)
		if err != nil {
			return agent.Transition{}, err
		}
		payload, err := json.Marshal(struct {
			Kind string `json:"kind"`
		}{Kind: crashTreeWaitPayloadKind})
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.RequestWait(key, payload)
		if err != nil {
			return agent.Transition{}, err
		}
		e.state.Phase = crashTreePhaseChildWaitOpening
		return agent.Continue(0, effect)
	case crashTreePhaseChildWaitOpening:
		if len(signals) != 1 {
			return agent.Transition{}, errors.New("agenttest: external wait acknowledgement is missing")
		}
		waitID, addressed := signals[0].WaitID()
		if !addressed {
			return agent.Transition{}, errors.New("agenttest: external wait acknowledgement has no WaitID")
		}
		e.state.WaitID = waitID.String()
		e.state.Phase = crashTreePhaseChildWaiting
		return agent.Wait(1, waitID)
	case crashTreePhaseChildWaiting:
		if len(signals) != 1 {
			return agent.Transition{}, errors.New("agenttest: external wait response is missing")
		}
		e.state.Phase = crashTreePhaseFinished
		output, err := agent.EncodeOutput(crashTreeOutput{Completed: true})
		if err != nil {
			return agent.Transition{}, err
		}
		return agent.Complete(1, output)
	default:
		return agent.Transition{}, errors.New("agenttest: child cannot advance from its current phase")
	}
}

func (e *crashTreeExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(e.state)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState(e.definition.descriptor.Name(), payload)
}

type crashTreeDispatcher struct{}

func (crashTreeDispatcher) Dispatch(
	context.Context,
	agent.EffectRequest,
	agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, errors.New("agenttest: crash tree has no Dispatcher Effects")
}

func (crashTreeDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy {
	return agent.ReplayPolicyNever
}

func testCrashAfterAdministrativeCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	parkedGate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitCheckpointParked, phase: crashCommitAfter,
	})
	deployment := newCrashTreeDeployment(t)
	engine := newCrashEngine(t, parkedGate, nil)
	root := startCrashTree(t, engine, deployment)
	parked := parkedGate.await(t)
	parkedGate.continueCommit()
	waitForConformanceStatus(t, root, agent.StatusWaiting)
	childID := crashTreeChildID(t, parked.prospective, root.ID())
	child, found := engine.Process(childID)
	if !found {
		t.Fatal("waiting child was not published")
	}
	waitForConformanceStatus(t, child, agent.StatusWaiting)

	prepared, err := engine.PrepareWaitingSubtreeCancellation(
		t.Context(), root.ID(), child.ID(), crashTreeCancellationReason,
	)
	if err != nil {
		t.Fatal(err)
	}
	adminGate := newAdministrativeCommitGate(
		t, store, prepared.SourceTreeDigest(), prepared.ResultingSnapshot(),
	)
	commitResult := make(chan error, 1)
	go func() { commitResult <- adminGate.commit() }()
	adminGate.await(t)
	head := assertCrashHead(
		t, store, root.ID(), prepared.ResultingSnapshot().Digest(),
	)
	if root.Status() != agent.StatusWaiting || child.Status() != agent.StatusWaiting {
		t.Fatal("administrative state was applied in memory before Apply")
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restoredRoot := restoreCrashTree(t, restoredEngine, deployment, head)
	if restoredRoot.Status() != agent.StatusPaused {
		t.Fatalf("restored root status=%s, want paused", restoredRoot.Status())
	}
	restoredChild, found := restoredEngine.Process(child.ID())
	if !found || restoredChild.Status() != agent.StatusCanceled {
		t.Fatalf(
			"restored child found=%t status=%s, want canceled",
			found, restoredChild.Status(),
		)
	}
	adminGate.abort()
	if err := awaitAdministrativeCommit(t, commitResult); !errors.Is(err, errSimulatedHostCrash) {
		t.Fatalf("administrative commit result=%v", err)
	}
	if err := prepared.Discard(); err != nil {
		t.Fatal(err)
	}

	if err := restoredRoot.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	if result := awaitCrashProcess(t, restoredRoot); result.Status() != agent.StatusCompleted {
		t.Fatalf("restored root result=%s", result.Status())
	}
	closeCrashEngine(t, restoredEngine)
	finishCrashProcess(t, root)
	awaitCrashProcess(t, child)
	closeCrashEngine(t, engine)
}

func newCrashTreeDeployment(t *testing.T) agent.Deployment {
	t.Helper()
	inputSchema, err := agent.SchemaFor[crashTreeInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := agent.SchemaFor[crashTreeOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: crashTreeDeploymentName, Description: crashTreeDeploymentDescription,
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := &crashTreeDefinition{descriptor: descriptor}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: crashTreeDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte(crashTreeImplementationSeed)),
		ConfigurationDigest:  agent.ComputeDigest([]byte(crashTreeConfigurationSeed)),
	})
	if err != nil {
		t.Fatal(err)
	}
	definition.reference = deployment.DeploymentRef()
	return deployment
}

func startCrashTree(
	t *testing.T,
	engine *agent.Engine,
	deployment agent.Deployment,
) *agent.Process {
	t.Helper()
	input, err := deployment.Descriptor().EncodeInput(crashTreeInput{Role: crashTreeRoleRoot})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Start(t.Context(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	return process
}

func crashTreeChildID(
	t *testing.T,
	snapshot agent.TreeSnapshot,
	rootID agent.ProcessID,
) agent.ProcessID {
	t.Helper()
	for _, process := range snapshot.ProcessSnapshots() {
		if process.ProcessID() != rootID &&
			process.Relation().Depth() == crashTreeDirectChildDepth {
			return process.ProcessID()
		}
	}
	t.Fatal("parked tree has no direct child")
	return agent.ProcessID{}
}

func awaitAdministrativeCommit(t *testing.T, result <-chan error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		t.Fatalf("administrative commit did not return: %v", ctx.Err())
		return fmt.Errorf("agenttest: administrative commit wait: %w", ctx.Err())
	}
}
