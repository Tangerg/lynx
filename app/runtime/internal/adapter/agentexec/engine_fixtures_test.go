package agentexec

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// newHistoryStore re-exports history.NewInMemoryStore under a
// shorter test-only name so the persistent-store test reads as
// "shared history store".
func newHistoryStore() history.Store { return history.NewInMemoryStore() }

type assembledEngine struct {
	*Engine
	catalog *toolset.Resolver
	closers []func() error
}

func (e *assembledEngine) Close() error {
	var errs []error
	for index := len(e.closers) - 1; index >= 0; index-- {
		if closeFn := e.closers[index]; closeFn != nil {
			errs = append(errs, closeFn())
		}
	}
	return errors.Join(errs...)
}

// mustEngineWith builds an engine over a tool environment assembled by
// toolset.Build (the production path: capabilities + resolver constructed
// outside the core, injected in) -- for tests that exercise the assembled tool
// set.
func mustEngineWith(t *testing.T, client *chatclient.Client, bc toolset.BuildConfig) *assembledEngine {
	t.Helper()
	built, err := toolset.Build(context.Background(), bc)
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	eng, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
	})
	if err != nil {
		for index := len(built.Closers) - 1; index >= 0; index-- {
			if closeFn := built.Closers[index]; closeFn != nil {
				_ = closeFn()
			}
		}
		t.Fatalf("engine.New: %v", err)
	}
	return &assembledEngine{
		Engine:  eng,
		catalog: built.Resolver,
		closers: built.Closers,
	}
}

func codingTools(t *testing.T, resolver *toolset.Resolver) []tools.Tool {
	t.Helper()
	group, ok, err := resolver.Resolve(t.Context(), toolport.ToolRoleCoding)
	if err != nil || !ok {
		t.Fatalf("Resolve(coding) = %v, %v", ok, err)
	}
	values, err := group.Tools(t.Context())
	if err != nil {
		t.Fatalf("coding tools: %v", err)
	}
	return values
}

func cleanupBuiltTools(t *testing.T, built toolset.Built) {
	t.Helper()
	t.Cleanup(func() {
		for index := len(built.Closers) - 1; index >= 0; index-- {
			if closeFn := built.Closers[index]; closeFn != nil {
				_ = closeFn()
			}
		}
	})
}

func (e *Engine) runTurnSync(ctx context.Context, req TurnRequest) (TurnOutput, error) {
	proc, err := e.StartTurn(ctx, req)
	if err != nil {
		return TurnOutput{}, fmt.Errorf("engine: start turn: %w", err)
	}
	completion := proc.Await()
	if err := completion.Err; err != nil {
		return TurnOutput{}, fmt.Errorf("engine: run turn: %w", err)
	}
	if !completion.HasOutput {
		return TurnOutput{}, errors.New("engine: run turn: completed without output")
	}
	return completion.Output, nil
}

type startCall struct {
	process      ProcessRef
	callID       string
	sourceCallID string
	toolName     string
	arguments    string
}

type endCall struct {
	process      ProcessRef
	callID       string
	toolName     string
	arguments    string
	output       string
	mutatedPaths []string
	err          error
}

// recordingObserver collects every Start/End/Delta the engine fires
// so the test can assert on counts, ordering, and field values. Safe
// for concurrent use -- parallel tool calls would race the inner
// slices without the mutex.
type recordingObserver struct {
	mu              sync.Mutex
	startList       []startCall
	endList         []endCall
	deltaList       []string
	usageList       []usageObservation
	childCompletion []ChildCompletion
	order           []string
}

type usageObservation struct {
	process       ProcessRef
	usage         accounting.TokenUsage
	byModel       []accounting.ModelUsage
	costUSD       float64
	steps         int
	contextTokens int64
}

func (r *recordingObserver) ApproveToolCall(_ context.Context, _, _, _ string, _ ToolApprovalTarget) ToolApprovalVerdict {
	return ToolApprovalVerdict{} // auto-run; tests don't exercise the approval gate
}

func (r *recordingObserver) OnToolCallStart(process ProcessRef, callID, sourceCallID, toolName, arguments string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startList = append(r.startList, startCall{
		process: process, callID: callID, sourceCallID: sourceCallID, toolName: toolName, arguments: arguments,
	})
	r.order = append(r.order, "tool-start:"+process.ID+":"+toolName)
}

func (r *recordingObserver) OnToolCallEnd(process ProcessRef, callID, toolName, arguments, output string, _ *offload.Ref, mutatedPaths []string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endList = append(r.endList, endCall{
		process: process, callID: callID, toolName: toolName, arguments: arguments,
		output: output, mutatedPaths: mutatedPaths, err: err,
	})
	r.order = append(r.order, "tool-end:"+process.ID+":"+toolName)
}

func (r *recordingObserver) OnMessageDelta(_ ProcessRef, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltaList = append(r.deltaList, text)
}

// OnReasoningDelta is a no-op for the current tests -- reasoning
// streams aren't asserted at the engine level. Lyra-level tests
// in chat/impl_test.go cover the propagation path.
func (r *recordingObserver) OnReasoningDelta(ProcessRef, string) {}

func (r *recordingObserver) OnUsage(process ProcessRef, progress UsageProgress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usageList = append(r.usageList, usageObservation{
		process:       process,
		usage:         progress.Usage,
		byModel:       append([]accounting.ModelUsage(nil), progress.UsageByModel...),
		costUSD:       progress.CostUSD,
		steps:         progress.Steps,
		contextTokens: progress.ContextTokens,
	})
}

func (r *recordingObserver) OnChildProcessEnd(completion ChildCompletion) {
	r.mu.Lock()
	defer r.mu.Unlock()
	completion.UsageByModel = append([]accounting.ModelUsage(nil), completion.UsageByModel...)
	r.childCompletion = append(r.childCompletion, completion)
	r.order = append(r.order, "child-end:"+completion.Process.ID)
}

func (r *recordingObserver) starts() []startCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]startCall, len(r.startList))
	copy(out, r.startList)
	return out
}

func (r *recordingObserver) ends() []endCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]endCall, len(r.endList))
	copy(out, r.endList)
	return out
}

func (r *recordingObserver) deltas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.deltaList))
	copy(out, r.deltaList)
	return out
}

func (r *recordingObserver) usages() []usageObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]usageObservation(nil), r.usageList...)
}

func (r *recordingObserver) childCompletions() []ChildCompletion {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ChildCompletion(nil), r.childCompletion...)
}

func (r *recordingObserver) eventOrder() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

type hitlApprovalObserver struct {
	recordingObserver
}

func (o *hitlApprovalObserver) ApproveToolCall(ctx context.Context, _, toolName, arguments string, _ ToolApprovalTarget) ToolApprovalVerdict {
	pending := runs.Interrupt{
		Kind: execution.ApprovalInterrupt,
		Approval: &runs.ApprovalPrompt{
			ToolName: toolName, Arguments: arguments, SafetyClass: tool.SafetyClassExec,
		},
	}
	res, err := suspension.Interrupt(ctx,
		interrupts.InterruptKey("kernel-test.approval", toolName, arguments),
		pending,
	)
	if err != nil {
		return ToolApprovalVerdict{Interrupt: err}
	}
	if !res.Approved {
		return ToolApprovalVerdict{Denied: true, DenyReason: "denied"}
	}
	return ToolApprovalVerdict{Arguments: res.Arguments}
}

type jsonProcessStore struct {
	mu          sync.Mutex
	snapshots   map[string]execution.ProcessState
	checkpoints map[string]execution.ProcessCheckpoint
}

func newJSONProcessStore() *jsonProcessStore {
	return &jsonProcessStore{
		snapshots:   map[string]execution.ProcessState{},
		checkpoints: map[string]execution.ProcessCheckpoint{},
	}
}

func (s *jsonProcessStore) SaveTree(_ context.Context, tree execution.ProcessTreeState, checkpoint execution.ProcessCheckpoint) error {
	if err := tree.Validate(); err != nil {
		return err
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	prepared := make(map[string]execution.ProcessState, len(tree.Processes))
	for _, process := range tree.Processes {
		process.Payload = append([]byte(nil), process.Payload...)
		prepared[process.ID] = process
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.deleteTree(tree.RootID); err != nil {
		return err
	}
	for id, process := range prepared {
		s.snapshots[id] = process
	}
	s.checkpoints[tree.RootID] = checkpoint
	return nil
}

func (s *jsonProcessStore) LoadTree(_ context.Context, id string) (execution.ProcessTreeState, execution.ProcessCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.snapshots[id]
	if !ok {
		return execution.ProcessTreeState{}, execution.ProcessCheckpoint{}, fmt.Errorf("json process store: load %q: %w", id, execution.ErrProcessStateNotFound)
	}
	children := make(map[string][]string)
	for processID, process := range s.snapshots {
		children[process.ParentID] = append(children[process.ParentID], processID)
	}
	var processes []execution.ProcessState
	var collect func(string)
	collect = func(processID string) {
		process := s.snapshots[processID]
		process.Payload = append([]byte(nil), process.Payload...)
		processes = append(processes, process)
		for _, childID := range children[processID] {
			collect(childID)
		}
	}
	collect(id)
	checkpoint := s.checkpoints[id]
	return execution.ProcessTreeState{RootID: id, Processes: processes}, checkpoint, nil
}

func (s *jsonProcessStore) List(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.snapshots))
	for id := range s.snapshots {
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *jsonProcessStore) DeleteTrees(_ context.Context, rootIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rootID := range rootIDs {
		if err := s.deleteTree(rootID); err != nil {
			return err
		}
	}
	return nil
}

func (s *jsonProcessStore) deleteTree(rootID string) error {
	children := make(map[string][]string)
	for id, process := range s.snapshots {
		if process.ParentID != "" {
			children[process.ParentID] = append(children[process.ParentID], id)
		}
	}
	var remove func(string)
	remove = func(id string) {
		delete(s.snapshots, id)
		delete(s.checkpoints, id)
		for _, childID := range children[id] {
			remove(childID)
		}
	}
	remove(rootID)
	return nil
}

type optionToolStub struct {
	defaults *chat.Options

	mu          sync.Mutex
	lastOptions *chat.Options
}

func newOptionToolStub() *optionToolStub {
	defaults := &chat.Options{Model: "stub-options-restore"}
	return &optionToolStub{defaults: defaults}
}

func (m *optionToolStub) DefaultOptions() chat.Options { return *m.defaults }

func (m *optionToolStub) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	m.capture(req)
	if hasToolMessage(req.Messages) {
		return responseWithText("restored ok")
	}
	return responseWithToolCall("shell", `{"command":"echo lyra"}`)
}

func (m *optionToolStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

func (m *optionToolStub) capture(req *chat.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := req.Options.Clone()
	m.lastOptions = &copy
}

func (m *optionToolStub) lastCapturedOptions() *chat.Options {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastOptions == nil {
		return nil
	}
	copy := m.lastOptions.Clone()
	return &copy
}

// namedUsageStub reports a configurable served-model name (and 1/1 usage) in
// a single round -- used to detect which client a turn actually ran against.
type namedUsageStub struct {
	model    string
	defaults *chat.Options
}

func newNamedStub(model string) *namedUsageStub {
	opts := &chat.Options{Model: model}
	return &namedUsageStub{model: model, defaults: opts}
}

func (m *namedUsageStub) DefaultOptions() chat.Options { return *m.defaults }

func (m *namedUsageStub) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewTextPart("ok"))
	resp, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	if resp != nil {
		resp.Usage = chat.Usage{InputTokens: 1, OutputTokens: 1}
		resp.Model = m.model
	}
	return resp, err
}

func (m *namedUsageStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}
