package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
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
	if err := <-proc.Done(); err != nil {
		return TurnOutput{}, fmt.Errorf("engine: run turn: %w", err)
	}
	return proc.Output()
}

type startCall struct {
	callID    string
	toolName  string
	arguments string
}

type endCall struct {
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
	mu        sync.Mutex
	startList []startCall
	endList   []endCall
	deltaList []string
}

func (r *recordingObserver) ApproveToolCall(_ context.Context, _, _, _ string, _ ToolApprovalTarget) ToolApprovalVerdict {
	return ToolApprovalVerdict{} // auto-run; tests don't exercise the approval gate
}

func (r *recordingObserver) OnToolCallStart(callID, toolName, arguments string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startList = append(r.startList, startCall{callID, toolName, arguments})
}

func (r *recordingObserver) OnToolCallEnd(callID, toolName, arguments, output string, _ *offload.Ref, mutatedPaths []string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endList = append(r.endList, endCall{
		callID: callID, toolName: toolName, arguments: arguments,
		output: output, mutatedPaths: mutatedPaths, err: err,
	})
}

func (r *recordingObserver) OnMessageDelta(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltaList = append(r.deltaList, text)
}

// OnReasoningDelta is a no-op for the current tests -- reasoning
// streams aren't asserted at the engine level. Lyra-level tests
// in chat/impl_test.go cover the propagation path.
func (r *recordingObserver) OnReasoningDelta(_ string) {}

// OnUsage is a no-op here -- the mid-run usage signal is asserted at the
// transport layer (translator_test.go), not the engine level.
func (r *recordingObserver) OnUsage(accounting.TokenUsage, float64, int64) {}

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

type hitlApprovalObserver struct {
	recordingObserver
}

func (o *hitlApprovalObserver) ApproveToolCall(ctx context.Context, _, toolName, arguments string, _ ToolApprovalTarget) ToolApprovalVerdict {
	res, err := hitl.Interrupt[interrupts.Resolution](ctx,
		interrupts.InterruptKey("kernel-test.approval", toolName, arguments),
		map[string]string{"tool": toolName, "arguments": arguments},
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
	mu        sync.Mutex
	snapshots map[string]json.RawMessage
}

func newJSONProcessStore() *jsonProcessStore {
	return &jsonProcessStore{snapshots: map[string]json.RawMessage{}}
}

func (s *jsonProcessStore) Apply(_ context.Context, change core.ProcessSnapshotChange) error {
	if err := change.Validate(); err != nil {
		return err
	}
	prepared := make(map[string]json.RawMessage)
	if change.Tree != nil {
		prepared = make(map[string]json.RawMessage, len(change.Tree.Snapshots))
		for _, snapshot := range change.Tree.Snapshots {
			raw, err := json.Marshal(snapshot)
			if err != nil {
				return err
			}
			prepared[snapshot.ID] = raw
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, raw := range prepared {
		s.snapshots[id] = raw
	}
	for _, rootID := range change.DeleteRoots {
		if err := s.deleteTree(rootID); err != nil {
			return err
		}
	}
	return nil
}

func (s *jsonProcessStore) Load(_ context.Context, id string) (core.ProcessSnapshot, error) {
	s.mu.Lock()
	raw, ok := s.snapshots[id]
	s.mu.Unlock()
	if !ok {
		return core.ProcessSnapshot{}, fmt.Errorf("json process store: load %q: %w", id, core.ErrSnapshotNotFound)
	}
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return core.ProcessSnapshot{}, err
	}
	return snapshot, nil
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

func (s *jsonProcessStore) deleteTree(rootID string) error {
	children := make(map[string][]string)
	for id, raw := range s.snapshots {
		var snapshot core.ProcessSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return fmt.Errorf("json process store: decode %q for delete: %w", id, err)
		}
		if snapshot.ParentID != "" {
			children[snapshot.ParentID] = append(children[snapshot.ParentID], id)
		}
	}
	var remove func(string)
	remove = func(id string) {
		delete(s.snapshots, id)
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
