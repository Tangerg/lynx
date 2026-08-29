package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

type crashCommitKind uint8

const (
	crashCommitInvalid crashCommitKind = iota
	crashCommitRootOutcome
	crashCommitActivation
	crashCommitEffectPending
	crashCommitEffectSettled
	crashCommitCheckpointParked
	crashCommitCheckpointTerminal
)

type crashCommitPhase uint8

const (
	crashCommitPhaseInvalid crashCommitPhase = iota
	crashCommitBefore
	crashCommitAfter
)

type crashCommitPoint struct {
	kind  crashCommitKind
	phase crashCommitPhase
}

func (p crashCommitPoint) valid() bool {
	return p.kind > crashCommitInvalid && p.kind <= crashCommitCheckpointTerminal &&
		(p.phase == crashCommitBefore || p.phase == crashCommitAfter)
}

type crashCommitObservation struct {
	rootID         agent.ProcessID
	previousDigest agent.Digest
	prospective    agent.TreeSnapshot
}

var errSimulatedHostCrash = errors.New("agenttest: simulated host crash")

// treeDurabilityCommitGate cuts the callback itself, not an Engine goroutine.
// An after cut occurs after the delegate has advanced its authoritative head
// and before the callback can return to the Runtime for in-memory apply.
type treeDurabilityCommitGate struct {
	delegate agent.TreeDurability
	point    crashCommitPoint

	mu      sync.Mutex
	claimed bool

	reached  chan crashCommitObservation
	decision chan error
	resolve  sync.Once
}

func newTreeDurabilityCommitGate(
	t *testing.T,
	delegate agent.TreeDurability,
	point crashCommitPoint,
) *treeDurabilityCommitGate {
	t.Helper()
	if delegate == nil || !point.valid() {
		t.Fatal("invalid tree durability crash gate")
	}
	gate := &treeDurabilityCommitGate{
		delegate: delegate,
		point:    point,
		reached:  make(chan crashCommitObservation, 1),
		decision: make(chan error, 1),
	}
	t.Cleanup(func() { gate.abort() })
	return gate
}

func (g *treeDurabilityCommitGate) AcknowledgeProcessStartOutcome(
	ctx context.Context,
	outcome agent.ProcessStartOutcome,
) error {
	snapshot, _ := outcome.TreeSnapshot()
	previous, _ := outcome.PreviousTreeDigest()
	observation := crashCommitObservation{
		rootID: outcome.Admission().Relation().RootID(), previousDigest: previous,
		prospective: snapshot,
	}
	kind := crashCommitInvalid
	if outcome.Admission().Relation().IsRoot() {
		kind = crashCommitRootOutcome
	}
	point := crashCommitPoint{kind: kind, phase: g.point.phase}
	return g.around(point, observation, func() error {
		return g.delegate.AcknowledgeProcessStartOutcome(ctx, outcome)
	})
}

func (g *treeDurabilityCommitGate) ActivateTree(
	ctx context.Context,
	activation agent.TreeActivation,
) error {
	observation := crashCommitObservation{
		rootID:         activation.TreeSnapshot().RootID(),
		previousDigest: activation.PreviousTreeDigest(),
		prospective:    activation.TreeSnapshot(),
	}
	point := crashCommitPoint{kind: crashCommitActivation, phase: g.point.phase}
	return g.around(point, observation, func() error {
		return g.delegate.ActivateTree(ctx, activation)
	})
}

func (g *treeDurabilityCommitGate) CommitEffect(
	ctx context.Context,
	boundary agent.EffectBoundary,
) error {
	kind := crashCommitInvalid
	switch boundary.Kind() {
	case agent.EffectBoundaryPending:
		kind = crashCommitEffectPending
	case agent.EffectBoundarySettled:
		kind = crashCommitEffectSettled
	}
	observation := crashCommitObservation{
		rootID:         boundary.TreeSnapshot().RootID(),
		previousDigest: boundary.PreviousTreeDigest(),
		prospective:    boundary.TreeSnapshot(),
	}
	point := crashCommitPoint{kind: kind, phase: g.point.phase}
	return g.around(point, observation, func() error {
		return g.delegate.CommitEffect(ctx, boundary)
	})
}

func (g *treeDurabilityCommitGate) CommitCheckpoint(
	ctx context.Context,
	checkpoint agent.TreeCheckpoint,
) error {
	kind := crashCommitInvalid
	switch checkpoint.Kind() {
	case agent.TreeCheckpointParked:
		kind = crashCommitCheckpointParked
	case agent.TreeCheckpointTerminal:
		kind = crashCommitCheckpointTerminal
	}
	observation := crashCommitObservation{
		rootID:         checkpoint.TreeSnapshot().RootID(),
		previousDigest: checkpoint.PreviousTreeDigest(),
		prospective:    checkpoint.TreeSnapshot(),
	}
	point := crashCommitPoint{kind: kind, phase: g.point.phase}
	return g.around(point, observation, func() error {
		return g.delegate.CommitCheckpoint(ctx, checkpoint)
	})
}

func (g *treeDurabilityCommitGate) around(
	point crashCommitPoint,
	observation crashCommitObservation,
	commit func() error,
) error {
	if point.kind != g.point.kind {
		return commit()
	}
	if point.phase == crashCommitBefore && g.claim() {
		if err := g.cut(observation); err != nil {
			return err
		}
	}
	if err := commit(); err != nil {
		return err
	}
	if point.phase == crashCommitAfter && g.claim() {
		return g.cut(observation)
	}
	return nil
}

func (g *treeDurabilityCommitGate) claim() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.claimed {
		return false
	}
	g.claimed = true
	return true
}

func (g *treeDurabilityCommitGate) cut(observation crashCommitObservation) error {
	g.reached <- observation
	return <-g.decision
}

func (g *treeDurabilityCommitGate) await(t *testing.T) crashCommitObservation {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case observation := <-g.reached:
		return observation
	case <-ctx.Done():
		t.Fatalf("durability gate was not reached: %v", ctx.Err())
		return crashCommitObservation{}
	}
}

func (g *treeDurabilityCommitGate) continueCommit() {
	g.resolve.Do(func() { g.decision <- nil })
}

func (g *treeDurabilityCommitGate) abort() {
	g.resolve.Do(func() { g.decision <- errSimulatedHostCrash })
}

type crashStartResult struct {
	process *agent.Process
	err     error
}

type crashRestoreResult struct {
	process *agent.Process
	err     error
}

type crashAwaitResult struct {
	result agent.Result
	err    error
}

const (
	crashDeploymentName        = "agenttest.durability_crash"
	crashDeploymentDescription = "Exercises exact durable crash prefixes."
	crashDeploymentVersion     = "1.0.0"
	crashImplementationSeed    = "agenttest durability crash implementation v1"
	crashConfigurationSeed     = "agenttest durability crash configuration v1"
	crashInputValue            = "crash-prefix"
	crashCleanupReason         = "durability crash matrix cleanup"
)

func TestTreeDurabilityCrashPrefixMatrix(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "root outcome before commit", run: testCrashBeforeRootOutcomeCommit},
		{name: "root outcome after commit before Process publication", run: testCrashAfterRootOutcomeCommit},
		{name: "pending before commit", run: testCrashBeforePendingCommit},
		{name: "pending after commit before dispatch", run: testCrashAfterPendingCommit},
		{name: "after dispatch before settled commit", run: testCrashBeforeSettledCommit},
		{name: "settled after commit before memory apply", run: testCrashAfterSettledCommit},
		{name: "parked after commit before Event publication", run: testCrashAfterParkedCommit},
		{name: "terminal after commit before Result publication", run: testCrashAfterTerminalCommit},
		{name: "activation after CAS before Process publication", run: testCrashAfterActivationCommit},
		{name: "admin after product commit before Apply", run: testCrashAfterAdministrativeCommit},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func testCrashBeforeRootOutcomeCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitRootOutcome, phase: crashCommitBefore,
	})
	deployment, _ := newCrashDeployment(t, conformanceModePause, agent.ReplayPolicyNever)
	engine := newCrashEngine(t, gate, nil)
	started := startCrashProcessAsync(t, engine, deployment)
	observation := gate.await(t)
	assertCrashHeadAbsent(t, store, observation.rootID)
	if _, found := engine.Process(observation.rootID); found {
		t.Fatal("root Process published before its base head commit")
	}
	gate.abort()
	result := awaitCrashStart(t, started)
	if !errors.Is(result.err, errSimulatedHostCrash) || result.process != nil {
		t.Fatalf("Start result Process=%v error=%v", result.process != nil, result.err)
	}
	closeCrashEngine(t, engine)
}

func testCrashAfterRootOutcomeCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitRootOutcome, phase: crashCommitAfter,
	})
	deployment, _ := newCrashDeployment(t, conformanceModePause, agent.ReplayPolicyNever)
	engine := newCrashEngine(t, gate, nil)
	started := startCrashProcessAsync(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	if _, found := engine.Process(observation.rootID); found {
		t.Fatal("root Process published before its committed base callback returned")
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	waitForConformanceStatus(t, restored, agent.StatusPaused)
	gate.abort()
	result := awaitCrashStart(t, started)
	if !errors.Is(result.err, errSimulatedHostCrash) || result.process != nil {
		t.Fatalf("Start result Process=%v error=%v", result.process != nil, result.err)
	}
	finishCrashProcess(t, restored)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashBeforePendingCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitEffectPending, phase: crashCommitBefore,
	})
	deployment, dispatcher := newCrashDeployment(
		t, conformanceModeEffect, agent.ReplayPolicyNever,
		crashSucceededDispatchStep(t),
	)
	engine := newCrashEngine(t, gate, nil)
	original := startCrashProcess(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.previousDigest)
	if len(dispatcher.Requests()) != 0 {
		t.Fatal("Dispatcher ran before pending state became authoritative")
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	result := awaitCrashProcess(t, restored)
	if result.Status() != agent.StatusCompleted || len(dispatcher.Requests()) != 1 {
		t.Fatalf("recomputed result=%s dispatches=%d", result.Status(), len(dispatcher.Requests()))
	}
	gate.abort()
	awaitCrashProcess(t, original)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashAfterPendingCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitEffectPending, phase: crashCommitAfter,
	})
	deployment, dispatcher := newCrashDeployment(
		t, conformanceModeEffect, agent.ReplayPolicyNever,
	)
	engine := newCrashEngine(t, gate, nil)
	original := startCrashProcess(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	if len(dispatcher.Requests()) != 0 {
		t.Fatal("Dispatcher ran before the pending callback returned")
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	resolveCrashUnknown(t, restored)
	if result := awaitCrashProcess(t, restored); result.Status() != agent.StatusCompleted {
		t.Fatalf("resolved result=%s", result.Status())
	}
	if len(dispatcher.Requests()) != 0 {
		t.Fatalf("never-replay pending Effect dispatches=%d", len(dispatcher.Requests()))
	}
	gate.abort()
	awaitCrashProcess(t, original)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashBeforeSettledCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitEffectSettled, phase: crashCommitBefore,
	})
	deployment, dispatcher := newCrashDeployment(
		t, conformanceModeEffect, agent.ReplayPolicyNever,
		crashSucceededDispatchStep(t),
	)
	engine := newCrashEngine(t, gate, nil)
	original := startCrashProcess(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.previousDigest)
	if len(dispatcher.Requests()) != 1 {
		t.Fatalf("dispatches=%d, want 1", len(dispatcher.Requests()))
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	resolveCrashUnknown(t, restored)
	if result := awaitCrashProcess(t, restored); result.Status() != agent.StatusCompleted {
		t.Fatalf("resolved result=%s", result.Status())
	}
	if len(dispatcher.Requests()) != 1 {
		t.Fatalf("never-replay redispatched; calls=%d", len(dispatcher.Requests()))
	}
	gate.abort()
	awaitCrashProcess(t, original)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashAfterSettledCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitEffectSettled, phase: crashCommitAfter,
	})
	deployment, dispatcher := newCrashDeployment(
		t, conformanceModeEffect, agent.ReplayPolicyNever,
		crashSucceededDispatchStep(t),
	)
	engine := newCrashEngine(t, gate, nil)
	original := startCrashProcess(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	if original.Status().Terminal() {
		t.Fatal("settled state was applied in memory before its callback returned")
	}

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	if result := awaitCrashProcess(t, restored); result.Status() != agent.StatusCompleted {
		t.Fatalf("restored settled result=%s", result.Status())
	}
	if len(dispatcher.Requests()) != 1 {
		t.Fatalf("settled Effect redispatched; calls=%d", len(dispatcher.Requests()))
	}
	gate.abort()
	awaitCrashProcess(t, original)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashAfterParkedCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitCheckpointParked, phase: crashCommitAfter,
	})
	deployment, _ := newCrashDeployment(t, conformanceModePause, agent.ReplayPolicyNever)
	recorder := &ObservationRecorder{}
	engine := newCrashEngine(t, gate, recorder)
	original := startCrashProcess(t, engine, deployment)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	assertCrashEventAbsent(t, recorder, agent.EventProcessPaused)

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	if restored.Status() != agent.StatusPaused {
		t.Fatalf("restored status=%s, want paused", restored.Status())
	}
	gate.abort()
	awaitCrashProcess(t, original)
	finishCrashProcess(t, restored)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashAfterTerminalCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitCheckpointTerminal, phase: crashCommitAfter,
	})
	deployment, _ := newCrashDeployment(
		t, conformanceModeEffect, agent.ReplayPolicyNever,
		crashSucceededDispatchStep(t),
	)
	recorder := &ObservationRecorder{}
	engine := newCrashEngine(t, gate, recorder)
	original := startCrashProcess(t, engine, deployment)
	awaited := awaitCrashProcessAsync(original)
	observation := gate.await(t)
	head := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	select {
	case result := <-awaited:
		t.Fatalf("Result published before terminal callback returned: %+v", result)
	default:
	}
	assertCrashEventAbsent(t, recorder, agent.EventProcessFinished)

	restoredEngine := newCrashEngine(t, store, nil)
	restored := restoreCrashTree(t, restoredEngine, deployment, head)
	if result := awaitCrashProcess(t, restored); result.Status() != agent.StatusCompleted {
		t.Fatalf("restored terminal result=%s", result.Status())
	}
	gate.abort()
	awaitCrashAwait(t, awaited)
	closeCrashEngine(t, restoredEngine)
	closeCrashEngine(t, engine)
}

func testCrashAfterActivationCommit(t *testing.T) {
	store := NewMemoryTreeDurability()
	deployment, _ := newCrashDeployment(t, conformanceModePause, agent.ReplayPolicyNever)
	sourceEngine := newCrashEngine(t, store, nil)
	source := startCrashProcess(t, sourceEngine, deployment)
	head := waitForConformanceHeadStatus(t, store, source.ID(), agent.StatusPaused)

	gate := newTreeDurabilityCommitGate(t, store, crashCommitPoint{
		kind: crashCommitActivation, phase: crashCommitAfter,
	})
	firstEngine := newCrashEngine(t, gate, nil)
	firstRestore := restoreCrashTreeAsync(firstEngine, deployment, head)
	observation := gate.await(t)
	newHead := assertCrashHead(t, store, observation.rootID, observation.prospective.Digest())
	if _, found := firstEngine.Process(source.ID()); found {
		t.Fatal("restored Process published before activation callback returned")
	}

	secondEngine := newCrashEngine(t, store, nil)
	second := restoreCrashTree(t, secondEngine, deployment, newHead)
	if second.Status() != agent.StatusPaused {
		t.Fatalf("second restore status=%s, want paused", second.Status())
	}
	gate.abort()
	first := awaitCrashRestore(t, firstRestore)
	if !errors.Is(first.err, errSimulatedHostCrash) || first.process != nil {
		t.Fatalf("first restore Process=%v error=%v", first.process != nil, first.err)
	}
	closeCrashEngine(t, firstEngine)
	finishCrashProcess(t, second)
	closeCrashEngine(t, secondEngine)
	finishCrashProcess(t, source)
	closeCrashEngine(t, sourceEngine)
}

func newCrashDeployment(
	t *testing.T,
	mode conformanceMode,
	replayPolicy agent.ReplayPolicy,
	steps ...DispatchStep,
) (agent.Deployment, *ScriptedDispatcher) {
	t.Helper()
	inputSchema, err := agent.SchemaFor[conformanceInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := agent.SchemaFor[conformanceOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: crashDeploymentName, Description: crashDeploymentDescription,
		Version: crashDeploymentVersion, InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewScriptedDispatcher(ScriptedDispatcherConfig{
		ReplayPolicy: replayPolicy,
		Steps:        steps,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           &conformanceDefinition{descriptor: descriptor, mode: mode},
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte(crashImplementationSeed)),
		ConfigurationDigest:  agent.ComputeDigest([]byte(crashConfigurationSeed)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment, dispatcher
}

func crashSucceededDispatchStep(t *testing.T) DispatchStep {
	t.Helper()
	payload, err := json.Marshal(conformanceOutput{Value: crashInputValue})
	if err != nil {
		t.Fatal(err)
	}
	return DispatchStep{
		SettlementStatus:  agent.SettlementStatusSucceeded,
		SettlementPayload: payload,
	}
}

func newCrashEngine(
	t *testing.T,
	durability agent.TreeDurability,
	recorder *ObservationRecorder,
) *agent.Engine {
	t.Helper()
	config := agent.EngineConfig{TreeDurability: durability}
	if recorder != nil {
		config.EventListeners = []agent.EventListener{recorder}
	}
	engine, err := agent.NewEngine(config)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func startCrashProcess(
	t *testing.T,
	engine *agent.Engine,
	deployment agent.Deployment,
) *agent.Process {
	t.Helper()
	input, err := deployment.Descriptor().EncodeInput(conformanceInput{Value: crashInputValue})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Start(t.Context(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	return process
}

func startCrashProcessAsync(
	t *testing.T,
	engine *agent.Engine,
	deployment agent.Deployment,
) <-chan crashStartResult {
	t.Helper()
	input, err := deployment.Descriptor().EncodeInput(conformanceInput{Value: crashInputValue})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan crashStartResult, 1)
	go func() {
		process, startErr := engine.Start(t.Context(), deployment, input)
		result <- crashStartResult{process: process, err: startErr}
	}()
	return result
}

func restoreCrashTree(
	t *testing.T,
	engine *agent.Engine,
	deployment agent.Deployment,
	snapshot agent.TreeSnapshot,
) *agent.Process {
	t.Helper()
	process, err := engine.RestoreTree(t.Context(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return process
}

func restoreCrashTreeAsync(
	engine *agent.Engine,
	deployment agent.Deployment,
	snapshot agent.TreeSnapshot,
) <-chan crashRestoreResult {
	result := make(chan crashRestoreResult, 1)
	go func() {
		process, err := engine.RestoreTree(context.Background(), deployment, snapshot)
		result <- crashRestoreResult{process: process, err: err}
	}()
	return result
}

func assertCrashHeadAbsent(
	t *testing.T,
	store *MemoryTreeDurability,
	rootID agent.ProcessID,
) {
	t.Helper()
	_, exists, err := store.LoadTree(t.Context(), rootID)
	if err != nil || exists {
		t.Fatalf("authoritative head exists=%t error=%v, want absent", exists, err)
	}
}

func assertCrashHead(
	t *testing.T,
	store *MemoryTreeDurability,
	rootID agent.ProcessID,
	want agent.Digest,
) agent.TreeSnapshot {
	t.Helper()
	head, exists, err := store.LoadTree(t.Context(), rootID)
	if err != nil || !exists || !head.Valid() || head.Digest() != want {
		t.Fatalf(
			"authoritative head exists=%t valid=%t digest=%s want=%s error=%v",
			exists, head.Valid(), head.Digest(), want, err,
		)
	}
	return head
}

func assertCrashEventAbsent(
	t *testing.T,
	recorder *ObservationRecorder,
	name string,
) {
	t.Helper()
	for _, event := range recorder.Events() {
		if event.Name() == name {
			t.Fatalf("Event %s published before durable callback returned", name)
		}
	}
}

func resolveCrashUnknown(t *testing.T, process *agent.Process) {
	t.Helper()
	effectID := waitForConformanceUnknownEffect(t, process)
	payload, err := json.Marshal(conformanceOutput{Value: crashInputValue})
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := agent.NewSettlement(
		effectID, agent.SettlementStatusSucceeded, payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.ResolveUnknownEffect(t.Context(), settlement); err != nil {
		t.Fatal(err)
	}
}

func finishCrashProcess(t *testing.T, process *agent.Process) {
	t.Helper()
	if process == nil {
		return
	}
	if !process.Status().Terminal() {
		err := process.Kill(t.Context(), crashCleanupReason)
		if err != nil && !errors.Is(err, agent.ErrProcessFinished) {
			t.Fatalf("kill Process %s: %v", process.ID(), err)
		}
	}
	awaitCrashProcess(t, process)
}

func closeCrashEngine(t *testing.T, engine *agent.Engine) {
	t.Helper()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

func awaitCrashProcessAsync(process *agent.Process) <-chan crashAwaitResult {
	result := make(chan crashAwaitResult, 1)
	go func() {
		value, err := process.Await(context.Background())
		result <- crashAwaitResult{result: value, err: err}
	}()
	return result
}

func awaitCrashProcess(t *testing.T, process *agent.Process) agent.Result {
	t.Helper()
	return awaitCrashAwait(t, awaitCrashProcessAsync(process)).result
}

func awaitCrashAwait(t *testing.T, result <-chan crashAwaitResult) crashAwaitResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case value := <-result:
		if value.err != nil {
			t.Fatal(value.err)
		}
		return value
	case <-ctx.Done():
		t.Fatalf("Process did not settle: %v", ctx.Err())
		return crashAwaitResult{}
	}
}

func awaitCrashStart(t *testing.T, result <-chan crashStartResult) crashStartResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatalf("Engine.Start did not return: %v", ctx.Err())
		return crashStartResult{}
	}
}

func awaitCrashRestore(t *testing.T, result <-chan crashRestoreResult) crashRestoreResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), conformanceStatusTimeout)
	defer cancel()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatalf("Engine.RestoreTree did not return: %v", ctx.Err())
		return crashRestoreResult{}
	}
}
