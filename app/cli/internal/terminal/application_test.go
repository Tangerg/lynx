package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

func runUI(t *testing.T, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	backend := mock.New()
	backend.Instant = true
	return runUIWith(t, backend, plugins...)
}

var terminalControlSequence = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func showsPlain(t *testing.T, host *programtest.Host, expected string) {
	t.Helper()
	host.Until(t, "the interface to show "+expected, func() bool {
		if !host.Repaint() {
			return false
		}
		return strings.Contains(terminalControlSequence.ReplaceAllString(host.Frame(), ""), expected)
	})
}

func runUIWith(t *testing.T, backend agent.Runtime, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	return runUIWithWorkspace(t, backend, "/tmp/lyra-cli-test", plugins...)
}

func runUIWithWorkspace(t *testing.T, backend agent.Runtime, workspace string, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	return runUIConfigured(t, backend, workspace, nil, plugins...)
}

func runUIWithSettings(t *testing.T, backend agent.Runtime, configured settings.Config) (*programtest.Host, func()) {
	t.Helper()
	return runUIConfigured(t, backend, "/tmp/lyra-cli-test", &configured)
}

func runUIConfigured(t *testing.T, backend agent.Runtime, workspace string, configured *settings.Config, plugins ...extensions.Plugin) (*programtest.Host, func()) {
	t.Helper()
	return runUIFromConfig(t, Config{
		Runtime: backend, Workspace: workspace, Plugins: plugins, Settings: configured,
	})
}

func runUIFromConfig(t *testing.T, config Config) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	config.Host = host
	go func() {
		done <- Run(ctx, config)
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func runUIForSession(t *testing.T, backend agent.Runtime, sessionID string) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Runtime: backend, SessionID: sessionID, Host: host})
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

func runUIWithState(t *testing.T, backend agent.Runtime, workspace, sessionID, stateDirectory string) (*programtest.Host, func()) {
	t.Helper()
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: workspace, SessionID: sessionID,
			StateDirectory: stateDirectory, Host: host,
		})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop
}

type delayedFirstRuntime struct {
	*mock.Runtime

	starts atomic.Int32
}

type replayingResumeRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	attempts []agent.ResumeRun
	stream   agent.SegmentStream
}

type refusingFirstResumeRuntime struct {
	*mock.Runtime

	mu       sync.Mutex
	attempts []agent.ResumeRun
}

type recordingRuntime struct {
	*mock.Runtime

	mu     sync.Mutex
	last   agent.StartRun
	inputs []agent.StartRun
	starts int
}

type blockingSessionChangeRuntime struct {
	agent.Runtime

	creates       atomic.Int32
	blockCreateAt int32
	changeErr     error
	starts        atomic.Int32

	changeStarted chan struct{}
	releaseChange chan struct{}
	mu            sync.Mutex
	startedIn     string
}

type transientForkProjectionRuntime struct {
	*mock.Runtime

	forks     atomic.Int32
	remaining atomic.Int32
	mu        sync.Mutex
	forkedID  string
}

type flakyCancellationRuntime struct {
	agent.Runtime

	attempts  atomic.Int32
	remaining atomic.Int32
	failure   error
}

type refusingCloseCancellationRuntime struct {
	agent.Runtime
	canceled chan agent.CancelRun
	err      error
}

type invalidCloseCancellationRuntime struct {
	agent.Runtime
}

func (runtime *invalidCloseCancellationRuntime) CancelRun(
	context.Context,
	agent.CancelRun,
) (agent.RunCancellation, error) {
	return agent.RunCancellation{}, nil
}

func (runtime *refusingCloseCancellationRuntime) CancelRun(
	ctx context.Context,
	input agent.CancelRun,
) (agent.RunCancellation, error) {
	select {
	case runtime.canceled <- input:
	default:
	}
	return agent.RunCancellation{}, runtime.err
}

type mismatchedSessionUpdateRuntime struct {
	agent.Runtime
	returned agent.Session
}

func (r *mismatchedSessionUpdateRuntime) UpdateSession(ctx context.Context, input agent.UpdateSession) (agent.Session, error) {
	if _, err := r.Runtime.UpdateSession(ctx, input); err != nil {
		return agent.Session{}, err
	}
	return r.returned, nil
}

func (r *flakyCancellationRuntime) CancelRun(ctx context.Context, input agent.CancelRun) (agent.RunCancellation, error) {
	r.attempts.Add(1)
	for remaining := r.remaining.Load(); remaining > 0; remaining = r.remaining.Load() {
		if r.remaining.CompareAndSwap(remaining, remaining-1) {
			return agent.RunCancellation{}, r.failure
		}
	}
	return r.Runtime.CancelRun(ctx, input)
}

func (runtime *transientForkProjectionRuntime) ForkSession(ctx context.Context, input agent.ForkSession) (agent.Session, error) {
	forked, err := runtime.Runtime.ForkSession(ctx, input)
	if err != nil {
		return agent.Session{}, err
	}
	runtime.forks.Add(1)
	runtime.mu.Lock()
	runtime.forkedID = forked.ID
	runtime.mu.Unlock()
	return forked, nil
}

func (runtime *transientForkProjectionRuntime) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	runtime.mu.Lock()
	forkedID := runtime.forkedID
	runtime.mu.Unlock()
	if sessionID == forkedID {
		for remaining := runtime.remaining.Load(); remaining > 0; remaining = runtime.remaining.Load() {
			if runtime.remaining.CompareAndSwap(remaining, remaining-1) {
				return agent.SessionSnapshot{}, fmt.Errorf("temporary fork projection: %w", agent.ErrDisconnected)
			}
		}
	}
	return runtime.Runtime.GetSession(ctx, sessionID)
}

func (r *blockingSessionChangeRuntime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	ordinal := r.creates.Add(1)
	blockAt := r.blockCreateAt
	if blockAt == 0 {
		blockAt = 2
	}
	if ordinal != blockAt {
		return r.Runtime.CreateSession(ctx, input)
	}
	close(r.changeStarted)
	select {
	case <-r.releaseChange:
		if r.changeErr != nil {
			return agent.Session{}, r.changeErr
		}
		return r.Runtime.CreateSession(ctx, input)
	case <-ctx.Done():
		return agent.Session{}, context.Cause(ctx)
	}
}

func (r *blockingSessionChangeRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	r.starts.Add(1)
	r.mu.Lock()
	r.startedIn = input.SessionID
	r.mu.Unlock()
	return r.Runtime.StartRun(ctx, input)
}

func (r *blockingSessionChangeRuntime) startedSession() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.startedIn
}

func (r *recordingRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	r.mu.Lock()
	r.last = cloneStartRun(input)
	r.inputs = append(r.inputs, cloneStartRun(input))
	r.starts++
	r.mu.Unlock()
	return r.Runtime.StartRun(ctx, input)
}

func (r *recordingRuntime) startCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts
}

func (r *recordingRuntime) options() agent.RunOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last.Options
}

func (r *recordingRuntime) startInput() agent.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	input := r.last
	input.Message = input.Message.Clone()
	return input
}

func (r *recordingRuntime) startInputs() []agent.StartRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	inputs := make([]agent.StartRun, len(r.inputs))
	for index, input := range r.inputs {
		inputs[index] = cloneStartRun(input)
	}
	return inputs
}

func cloneStartRun(input agent.StartRun) agent.StartRun {
	input.Message = input.Message.Clone()
	return input
}

func (r *delayedFirstRuntime) StartRun(ctx context.Context, input agent.StartRun) (agent.SegmentStream, error) {
	if r.starts.Add(1) == 1 {
		<-ctx.Done()
		return agent.SegmentStream{}, context.Cause(ctx)
	}
	return r.Runtime.StartRun(ctx, input)
}

func (r *replayingResumeRuntime) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	r.mu.Lock()
	r.attempts = append(r.attempts, input)
	attempt := len(r.attempts)
	cached := r.stream
	r.mu.Unlock()
	if attempt == 1 {
		continued, err := r.Runtime.ResumeRun(ctx, input)
		if err != nil {
			return agent.SegmentStream{}, err
		}
		r.mu.Lock()
		r.stream = continued
		r.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("lost resume acknowledgement: %w", agent.ErrDisconnected)
	}
	return cached, nil
}

func (r *replayingResumeRuntime) resumeAttempts() []agent.ResumeRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.attempts)
}

func (r *refusingFirstResumeRuntime) ResumeRun(ctx context.Context, input agent.ResumeRun) (agent.SegmentStream, error) {
	r.mu.Lock()
	r.attempts = append(r.attempts, input)
	attempt := len(r.attempts)
	r.mu.Unlock()
	if attempt == 1 {
		return agent.SegmentStream{}, errors.New("answers rejected by runtime")
	}
	return r.Runtime.ResumeRun(ctx, input)
}

func (r *refusingFirstResumeRuntime) resumeAttempts() []agent.ResumeRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.attempts)
}

func TestMockConversationStreamsApprovalAndCompletes(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")

	host.Type("why is the cache test flaky?")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Shows(t, "How should lyra proceed?")
	host.Shows(t, "cache_test.go")

	host.Press(input.Enter)
	host.Shows(t, "complete")
	host.Shows(t, "Ran the test 50 times")
	host.Hides(t, "Tool approval")

	// A parked stream may finish at the same moment its continuation starts.
	// The second run proves that the retired stream cannot settle the new one.
	host.Type("run the analysis again")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Hides(t, "complete")
	host.Press(input.Esc)
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")
	host.Hides(t, "failed:")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalResumeRetriesTheSameMutationIdentity(t *testing.T) {
	base := mock.New()
	base.Instant = true
	runtime := &replayingResumeRuntime{Runtime: base}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("approval retry")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Enter)
	host.Shows(t, "complete")

	attempts := runtime.resumeAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID {
		t.Fatalf("resume attempts = %+v", attempts)
	}
	stop()
}

func TestRejectedApprovalResumePreservesTheReviewAndUsesANewIdentity(t *testing.T) {
	base := mock.New()
	base.Instant = true
	runtime := &refusingFirstResumeRuntime{Runtime: base}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("approval rejection")
	host.Press(input.Enter)
	showsPlain(t, host, "╭─Tool approval")
	for range 4 {
		host.Press(input.Down)
	}
	host.Press(input.Tab)
	host.Type("KEEP_REJECTED_REVIEW")
	host.Press(input.Enter)
	host.Until(t, "the refused resume to return ownership to the review", func() bool {
		return len(runtime.resumeAttempts()) == 1 && host.Repaint()
	})
	host.Shows(t, "KEEP_REJECTED_REVIEW")

	host.Press(input.Esc)
	host.Shows(t, "complete")
	attempts := runtime.resumeAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[1].CommandID == "" ||
		attempts[0].CommandID == attempts[1].CommandID {
		t.Fatalf("resume attempts = %+v", attempts)
	}
	answer, ok := attempts[1].Answers[0].Answer.(agent.ApprovalAnswer)
	if !ok || answer.Decision != agent.ApprovalDeny || answer.Reason != "KEEP_REJECTED_REVIEW" {
		t.Fatalf("retried approval answer = %#v", attempts[1].Answers[0].Answer)
	}
	stop()
}

func TestPendingApprovalResumeSurvivesRestartAndReplaysTheSameIdentity(t *testing.T) {
	base := mock.New()
	base.Instant = true
	opened, err := base.StartRun(t.Context(), agent.StartRun{
		CommandID: agent.CommandID("cli_55555555555555555555555555555555"),
		SessionID: "ses_demo_1", Message: agent.Message{Text: "persist approval delivery"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
	snapshot, err := base.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Interactions) != 1 {
		t.Fatalf("waiting interactions = %+v", snapshot.Interactions)
	}
	command := agent.ResumeRun{
		CommandID: agent.CommandID("cli_66666666666666666666666666666666"),
		RunID:     opened.RunID,
		Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(snapshot.Interactions[0]),
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove},
		}},
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StagePendingResume("ses_demo_1", workbench.PendingResume{
		Command: command, Interactions: snapshot.Interactions,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &replayingResumeRuntime{Runtime: base}
	host, stop := runUIWithState(t, runtime, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("resize during restored resume delivery failed")
	}
	host.Shows(t, "complete")
	attempts := runtime.resumeAttempts()
	if len(attempts) < 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[1].CommandID != command.CommandID {
		t.Fatalf("resume attempts after restart = %+v", attempts)
	}
	store, err = workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if pending, ok := store.PendingResume("ses_demo_1"); ok {
		t.Fatalf("acknowledged resume remains = %+v", pending)
	}
	stop()
}

func TestShiftEnterInsertsANewlineWithoutSubmitting(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Shows(t, "shift+enter")
	host.Type("first line")
	host.Send(input.Key{Code: input.Enter, Mods: input.Shift})
	host.Type("second line")
	host.Press(input.Enter)
	host.Shows(t, "complete")

	if got := backend.startInput().Message.Text; got != "first line\nsecond line" {
		t.Fatalf("submitted text = %q, want a two-line prompt", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestTranscriptReaderSearchesBeyondInlineToolSummary(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	lines := make([]string, maxToolDetailLines+60)
	for i := range lines {
		lines[i] = fmt.Sprintf("reader contract line %03d", i+1)
	}
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{
				ID: "long_tool", Kind: agent.BlockTool,
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Command: "long output", Status: agent.ToolOK,
					Output: strings.Join(lines, "\n"),
				},
			}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("show a long tool result")
	host.Press(input.Enter)
	host.Shows(t, "complete")

	host.Press(input.Tab)
	host.Send(input.Key{Code: input.Character, Rune: 'v'})
	host.Shows(t, "Reader")
	host.Shows(t, "long output")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("reader did not survive a temporarily minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("reader viewport could not be restored")
	}
	host.Shows(t, "Reader")
	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Ctrl})
	host.Shows(t, "Search reader")
	host.Type(lines[len(lines)-1])
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("reader search did not survive a temporarily minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("reader search viewport could not be restored")
	}
	host.Shows(t, lines[len(lines)-1])
	host.Press(input.Enter)
	host.Shows(t, "1 matches")
	host.Shows(t, lines[len(lines)-1])
	host.Press(input.Esc)
	host.Hides(t, "Reader")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestConfiguredKeySequencesDriveApplicationActions(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g s"}
	configured.Keys[settings.ActionSend] = []string{"g g"}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithSettings(t, backend, configured)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Send(input.Key{Code: input.Character, Rune: 's'})
	host.Shows(t, "Sessions")
	host.Press(input.Esc)
	host.Hides(t, "Sessions")

	host.Type("sequence submit")
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Shows(t, "complete")
	if got := backend.startInput().Message.Text; got != "sequence submit" {
		t.Fatalf("submitted text = %q, want the configured key sequence to submit the draft", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestConfiguredPrefixesRespectModalOwnershipAndResolveAfterTimeout(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionSessions] = []string{"g"}
	configured.Keys[settings.ActionShortcuts] = []string{"g s"}
	host, stop := runUIWithSettings(t, mock.New(), configured)
	host.Shows(t, "Ask lyra")

	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Ctrl})
	host.Shows(t, "Find in the live transcript")
	host.Type("g sequence")
	host.Shows(t, "g sequence")
	host.Hides(t, "Sessions")
	host.Press(input.Esc)
	host.Hides(t, "Find in the live transcript")

	// The exact g binding is also a prefix of g s, so it resolves through the
	// application clock only after the configured sequence timeout expires.
	host.Send(input.Key{Code: input.Character, Rune: 'g'})
	host.Shows(t, "Sessions")
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestTranscriptFocusDoesNotSubmitAndTypingReturnsToPrompt(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(prompt string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer-" + prompt, Kind: agent.BlockAssistant, Text: "focused answer · " + prompt}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("first prompt")
	host.Press(input.Enter)
	host.Shows(t, "focused answer · first prompt")
	host.Shows(t, "complete")

	host.Press(input.Tab)
	host.Shows(t, "select prev")
	host.Type("?")
	host.Shows(t, "Commands")
	host.Press(input.Esc)
	host.Hides(t, "Commands")
	host.Press(input.Enter)
	if got := backend.startCount(); got != 1 {
		t.Fatalf("Enter with transcript focus started %d runs, want 1", got)
	}

	host.Send(input.Paste{Text: "pasted prompt"})
	host.Shows(t, "pasted prompt")
	host.Press(input.Enter)
	host.Shows(t, "focused answer · pasted prompt")
	if got := backend.startCount(); got != 2 {
		t.Fatalf("pasting from transcript focus started %d runs, want 2", got)
	}

	host.Press(input.Tab)
	host.Shows(t, "select prev")
	host.Type("second prompt")
	host.Shows(t, "second prompt")
	host.Press(input.Enter)
	host.Shows(t, "focused answer · second prompt")
	if got := backend.startCount(); got != 3 {
		t.Fatalf("typing from transcript focus started %d runs, want 3", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCtrlCClearsTheDraftBeforeCancelingAnActiveRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("start a long run")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("UNSENT_CTRL_C_DRAFT")
	host.Shows(t, "UNSENT_CTRL_C_DRAFT")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Hides(t, "UNSENT_CTRL_C_DRAFT")
	host.Shows(t, "draft cleared; repeat ctrl+c to cancel")
	started := backend.startInput()
	snapshot, err := base.GetSession(t.Context(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); !active {
		t.Fatal("clear-first Ctrl+C canceled the active run")
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "canceled")
	stop()
}

func TestCancellationFailureLeavesTheRunRetryable(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &flakyCancellationRuntime{
		Runtime: base, failure: errors.New("temporary cancellation failure"),
	}
	backend.remaining.Store(1)
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("start a retryable run")
	host.Press(input.Enter)
	host.Shows(t, "working")

	host.Press(input.Esc)
	host.Shows(t, "could not cancel run: temporary cancellation failure")
	startedSession := firstRuntimeSession(t, base)
	snapshot, err := base.GetSession(t.Context(), startedSession)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); !active {
		t.Fatal("failed cancellation settled the runtime run")
	}

	host.Press(input.Esc)
	host.Shows(t, "canceled")
	if attempts := backend.attempts.Load(); attempts != 2 {
		t.Fatalf("cancellation attempts = %d, want 2", attempts)
	}
	stop()
}

func TestEscapeCancelsAnActiveRunWithoutDiscardingTheDraft(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, base)
	host.Shows(t, "Ask lyra")
	host.Type("start another long run")
	host.Press(input.Enter)
	host.Shows(t, "working")
	host.Type("PRESERVED_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Shows(t, "canceled")
	host.Shows(t, "PRESERVED_ESCAPE_DRAFT")
	stop()
}

func TestDoubleEscapeClearsAnIdleDraftAndKeepsItInHistory(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("RECOVERABLE_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Shows(t, "press Esc again to clear the draft")
	host.Shows(t, "RECOVERABLE_ESCAPE_DRAFT")
	host.Press(input.Esc)
	host.Hides(t, "RECOVERABLE_ESCAPE_DRAFT")
	host.Send(input.Key{Code: input.Up, Mods: input.Alt})
	host.Shows(t, "RECOVERABLE_ESCAPE_DRAFT")
	stop()
}

func TestQuitRequiresAConfirmingSecondPress(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: backend, Workspace: "/tmp/lyra-cli-test", Host: host,
		})
	}()

	host.Shows(t, "Ask lyra")
	quit := input.Key{Code: input.Character, Rune: 'q', Mods: input.Ctrl}
	host.Send(quit)
	host.Shows(t, "repeat ctrl+q or ctrl+d to quit")
	select {
	case err := <-done:
		t.Fatalf("first quit press stopped the app: %v", err)
	default:
	}

	host.Send(quit)
	wait, stopWaiting := context.WithTimeout(t.Context(), 2*time.Second)
	defer stopWaiting()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("confirmed quit returned an error: %v", err)
		}
	case <-wait.Done():
		t.Fatal("confirming quit press did not stop the app")
	}
}

func TestInteractiveRunRecoversTransportFaultsWithoutDuplicatingTranscript(t *testing.T) {
	for _, fault := range []mock.FaultKind{mock.FaultDisconnect, mock.FaultDuplicate} {
		t.Run(string(fault), func(t *testing.T) {
			backend := mock.New()
			backend.Instant = fault == mock.FaultDuplicate
			backend.Script = stableCompletedScript
			backend.Faults = []mock.SubscriptionFault{{Kind: fault, After: 1}}
			host, stop := runUIWith(t, backend)
			host.Shows(t, "Ask lyra")
			host.Type("recover the stream")
			host.Press(input.Enter)
			assertStableCompletedTranscript(t, host)
			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func TestInteractiveRunColdRecoversWhenTheDroppedSegmentAlreadyFinished(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	backend.Faults = []mock.SubscriptionFault{{Kind: mock.FaultDisconnect, After: 1}}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("recover from cold state")
	host.Press(input.Enter)
	assertStableCompletedTranscript(t, host)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func assertStableCompletedTranscript(t *testing.T, host *programtest.Host) {
	t.Helper()
	host.Shows(t, "complete")
	host.Hides(t, "failed:")
	host.Shows(t, "stable answer")
	if count := strings.Count(host.Frame(), "stable answer"); count != 1 {
		t.Fatalf("stable answer appears %d times:\n%s", count, host.Frame())
	}
}

func TestClosingTheTerminalCancelsTheOwnedRuntimeRun(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	backend := &recordingRuntime{Runtime: base}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("keep running until the terminal closes")
	host.Press(input.Enter)
	host.Shows(t, "keep running until")
	started := backend.startInput()
	if started.SessionID == "" {
		t.Fatal("run did not start")
	}
	stop()

	snapshot, err := base.GetSession(t.Context(), started.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active {
		t.Fatalf("terminal close left an active run: %+v", snapshot.Runs)
	}
	if snapshot.Session.Status != agent.SessionIdle {
		t.Fatalf("session status = %s, want idle", snapshot.Session.Status)
	}
}

func TestClosingTheTerminalPropagatesRuntimeCancellationFailure(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	want := errors.New("runtime rejected terminal close cancellation")
	runtime := &refusingCloseCancellationRuntime{
		Runtime: base, canceled: make(chan agent.CancelRun, 1), err: want,
	}
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: runtime, Workspace: "/tmp/lyra-cli-test", Host: host,
		})
	}()
	t.Cleanup(func() {
		cancel()
		_ = host.Close()
	})

	host.Shows(t, "Ask lyra")
	host.Type("keep running until close fails")
	host.Press(input.Enter)
	host.Shows(t, "keep running until")
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Run error = %v, want cancellation failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal did not return after cancellation failure")
	}
	request := awaitValue(t, runtime.canceled, "terminal-close cancellation")
	if request.RunID == "" || request.Reason != "terminal closed" {
		t.Fatalf("cancellation request = %+v", request)
	}
}

func TestClosingTheTerminalRejectsAnInvalidCancellationReceipt(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	runtime := &invalidCloseCancellationRuntime{Runtime: base}
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: runtime, Workspace: "/tmp/lyra-cli-test", Host: host,
		})
	}()
	t.Cleanup(func() {
		cancel()
		_ = host.Close()
	})

	host.Shows(t, "Ask lyra")
	host.Type("invalid close receipt")
	host.Press(input.Enter)
	host.Shows(t, "invalid close receipt")
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "validate terminal-close cancellation") {
			t.Fatalf("Run error = %v, want invalid cancellation receipt", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal did not return after invalid cancellation receipt")
	}
}

func TestClosingTheTerminalPropagatesFinalDraftPersistenceFailure(t *testing.T) {
	stateDirectory := t.TempDir()
	sessionsPath := filepath.Join(stateDirectory, "sessions")
	if err := os.MkdirAll(sessionsPath, 0o700); err != nil {
		t.Fatal(err)
	}
	base := mock.New()
	base.Instant = true
	host := programtest.New(t, 96, 28)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Runtime: base, Workspace: "/tmp/lyra-cli-test",
			StateDirectory: stateDirectory, Host: host,
		})
	}()
	t.Cleanup(func() {
		cancel()
		_ = os.Remove(sessionsPath)
		_ = host.Close()
	})

	host.Shows(t, "Ask lyra")
	if err := os.Remove(sessionsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionsPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	host.Type("draft whose final flush must fail")
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("Run error = %v, want final draft persistence failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("terminal did not return after draft persistence failure")
	}
}

func TestInteractiveRunRejectsConflictingReplay(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	backend.Faults = []mock.SubscriptionFault{{Kind: mock.FaultConflict, After: 1}}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("detect a conflict")
	host.Press(input.Enter)
	host.Shows(t, "failed:")
	host.Shows(t, "event identity conflict")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func stableCompletedScript(string) mock.Script {
	return mock.Script{Prelude: []mock.Step{
		{Delay: 30 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "stable answer"}}},
		{Delay: 100 * time.Millisecond, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
	}}
}

func TestDenyingApprovalIsAProductResult(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("do not change anything without asking")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSlashCompletionHelpAndTranscriptSearchUseRegisteredCommands(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/he")
	host.Shows(t, "show commands available")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Shows(t, "/find")
	host.Press(input.Esc)
	host.Hides(t, "Commands")

	host.Type("/find commands")
	host.Press(input.Enter)
	host.Shows(t, "match(es) for")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSlashCommandsEnforceTheirArgumentCardinality(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/status accidental")
	host.Press(input.Enter)
	host.Shows(t, "/status does not accept arguments")

	host.Type("/find")
	host.Press(input.Enter)
	host.Shows(t, "/find needs an argument")
	stop()
}

func TestAPluginCanAddACommandWithoutChangingTheShell(t *testing.T) {
	var loads atomic.Int32
	plugin := extensions.Plugin{ID: "test.greeting", Version: "1.0.0", APIVersion: extensions.HostAPIVersion, Capabilities: []extensions.Capability{SlashCommands.Capability()}, Setup: func(scope *extensions.Scope) error {
		loads.Add(1)
		_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
			Descriptor: CommandDescriptor{Name: "hello", Title: "run a contributed command"},
			Execute: func(context.Context, CommandRequest) (CommandResult, error) {
				return CommandResult{Message: fmt.Sprintf("hello from plugin load %d", loads.Load())}, nil
			},
		}, extensions.Contribution{})
		return err
	}}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 1")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Shows(t, "loaded   test.greeting@1.0.0")
	host.Type("/reload test.greeting")
	host.Press(input.Enter)
	host.Shows(t, "reloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 2")
	host.Type("/unload test.greeting")
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Shows(t, "unknown command: /hello")
	host.Type("/reload test.greeting")
	host.Press(input.Enter)
	host.Shows(t, "reloaded plugin test.greeting")
	host.Type("/hello")
	host.Press(input.Enter)
	host.Shows(t, "hello from plugin load 3")
	if got := loads.Load(); got != 3 {
		t.Fatalf("plugin setup ran %d times, want 3", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestAPluginSourceCanAddACommand(t *testing.T) {
	plugin := extensions.Plugin{
		ID: "test.source", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Descriptor: CommandDescriptor{Name: "source-command", Title: "run a source command"},
				Execute: func(context.Context, CommandRequest) (CommandResult, error) {
					return CommandResult{Message: "plugin source command complete"}, nil
				},
			}, extensions.Contribution{})
			return err
		},
	}
	backend := mock.New()
	backend.Instant = true
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: "/tmp/lyra-cli-test",
		PluginSources: []extensions.Source{extensions.StaticSource{Name: "test", Plugins: []extensions.Plugin{plugin}}},
	})
	host.Shows(t, "Ask lyra")
	host.Type("/source-command")
	host.Press(input.Enter)
	host.Shows(t, "plugin source command complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestAsynchronousPluginCommandKeepsTheTerminalResponsive(t *testing.T) {
	release := make(chan struct{})
	plugin := extensions.Plugin{
		ID: "test.async", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Descriptor: CommandDescriptor{Name: "slow", Title: "complete asynchronously"},
				Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
					select {
					case <-release:
						return CommandResult{Message: "async command complete"}, nil
					case <-ctx.Done():
						return CommandResult{}, context.Cause(ctx)
					}
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/slow")
	host.Press(input.Enter)
	host.Shows(t, "running /slow")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Shows(t, "test.async@1.0.0")
	close(release)
	host.Shows(t, "async command complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestPluginCommandPanicBecomesAnError(t *testing.T) {
	_, err := executeCommandSafely(t.Context(), SlashCommand{
		Descriptor: CommandDescriptor{Name: "boom", Title: "panic"},
		Execute:    func(context.Context, CommandRequest) (CommandResult, error) { panic("command boom") },
	}, CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "command boom") {
		t.Fatalf("command panic error = %v", err)
	}
}

func TestUnloadingPluginCancelsItsInFlightCommand(t *testing.T) {
	canceled := make(chan struct{}, 1)
	plugin := extensions.Plugin{
		ID: "test.cancel", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Descriptor: CommandDescriptor{Name: "wait", Title: "wait until unloaded"},
				Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
					<-ctx.Done()
					canceled <- struct{}{}
					return CommandResult{}, context.Cause(ctx)
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/wait")
	host.Press(input.Enter)
	host.Shows(t, "running /wait")
	host.Type("/unload test.cancel")
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.cancel")
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("plugin command was not canceled on unload")
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestUnloadingAPluginLeavesIndependentCommandsRunning(t *testing.T) {
	canceledA := make(chan struct{}, 1)
	canceledB := make(chan struct{}, 1)
	releaseB := make(chan struct{})
	commandPlugin := func(id, name string, canceled chan<- struct{}, release <-chan struct{}) extensions.Plugin {
		return extensions.Plugin{
			ID: id, Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
			Capabilities: []extensions.Capability{SlashCommands.Capability()},
			Setup: func(scope *extensions.Scope) error {
				_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
					Descriptor: CommandDescriptor{Name: name, Title: "wait for completion"},
					Execute: func(ctx context.Context, _ CommandRequest) (CommandResult, error) {
						select {
						case <-release:
							return CommandResult{Message: name + " complete"}, nil
						case <-ctx.Done():
							canceled <- struct{}{}
							return CommandResult{}, context.Cause(ctx)
						}
					},
				}, extensions.Contribution{})
				return err
			},
		}
	}
	pluginA := commandPlugin("test.cancel-a", "wait-a", canceledA, nil)
	pluginB := commandPlugin("test.cancel-b", "wait-b", canceledB, releaseB)
	host, stop := runUI(t, pluginA, pluginB)
	host.Shows(t, "Ask lyra")
	for _, command := range []string{"/wait-a", "/wait-b"} {
		host.Type(command)
		host.Press(input.Enter)
		host.Shows(t, "running "+command)
	}
	host.Type("/unload test.cancel-a")
	host.Press(input.Enter)
	host.Shows(t, "unloaded plugin test.cancel-a")
	select {
	case <-canceledA:
	case <-time.After(time.Second):
		t.Fatal("unloaded plugin command was not canceled")
	}
	select {
	case <-canceledB:
		t.Fatal("independent plugin command was canceled")
	default:
	}
	close(releaseB)
	host.Shows(t, "wait-b complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestPluginCommandCannotShadowABuiltin(t *testing.T) {
	plugin := extensions.Plugin{
		ID: "test.shadow", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := extensions.Contribute(scope, SlashCommands, SlashCommand{
				Descriptor: CommandDescriptor{Name: "help", Title: "shadow help"},
				Execute: func(context.Context, CommandRequest) (CommandResult, error) {
					return CommandResult{Message: "shadowed builtin"}, nil
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUI(t, plugin)
	host.Shows(t, "Ask lyra")
	host.Type("/help")
	host.Press(input.Enter)
	host.Shows(t, "/clear")
	host.Hides(t, "shadowed builtin")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionPickerRestoresHistoryAndLifecycleCommandsSwitchCleanly(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Shows(t, "Lyra CLI")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions")
	host.Type("Flaky cache")
	host.Shows(t, "Flaky cache expiry test")
	host.Press(input.Enter)
	host.Hides(t, "search sessions")
	host.Shows(t, "The fixed sleep races the janitor")
	host.Hides(t, "Lyra CLI")

	host.Type("/rename Restored cache investigation")
	host.Press(input.Enter)
	host.Shows(t, "renamed session to Restored cache investigation")

	host.Type("/fork Safe alternative")
	host.Press(input.Enter)
	host.Shows(t, "session · Safe alternative")
	host.Hides(t, "The fixed sleep races the janitor")
	host.Hides(t, "Lyra CLI")

	host.Type("/new")
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")
	host.Hides(t, "Lyra CLI")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestForkRetriesTheAuthoritativeReadWithoutRepeatingTheMutation(t *testing.T) {
	backend := &transientForkProjectionRuntime{Runtime: mock.New()}
	backend.remaining.Store(2)
	host, stop := runUIForSession(t, backend, "ses_demo_1")
	host.Shows(t, "Ask lyra")
	host.Type("/fork Recovered fork")
	host.Press(input.Enter)
	host.Shows(t, "session · Recovered fork")
	if forks := backend.forks.Load(); forks != 1 {
		t.Fatalf("fork mutations = %d, want exactly one", forks)
	}
	stop()
}

func TestSessionCenterPaginatesAndManagesSelectedSession(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	workspace := t.TempDir()
	var target agent.Session
	for index := range 30 {
		created, err := backend.CreateSession(t.Context(), agent.CreateSession{
			Title: fmt.Sprintf("Center target %02d", index), Workspace: workspace,
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			target = created
		}
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(target.Title)
	host.Shows(t, "0/20")
	host.Send(input.Key{Code: input.Character, Rune: 'l', Mods: input.Alt})
	host.Shows(t, target.Title)
	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	host.Shows(t, "Favorites · "+target.Title)

	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Alt})
	host.Shows(t, "Rename session")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("Renamed center target")
	host.Press(input.Enter)
	host.Hides(t, "Rename session")
	host.Shows(t, "Favorites · Renamed center target")

	host.Send(input.Key{Code: input.Character, Rune: 'd', Mods: input.Alt})
	host.Shows(t, "Delete session")
	for range 2 {
		host.Press(input.Down)
	}
	host.Press(input.Enter)
	host.Hides(t, "Delete session")
	host.Hides(t, "Renamed center target")
	if _, err := backend.GetSession(t.Context(), target.ID); !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("deleted session read error = %v", err)
	}

	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionCenterRejectsAMismatchedUpdateProjection(t *testing.T) {
	base := mock.New()
	workspace := t.TempDir()
	target, err := base.CreateSession(t.Context(), agent.CreateSession{Title: "Update target", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	returned, err := base.CreateSession(t.Context(), agent.CreateSession{Title: "Wrong response", Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	backend := &mismatchedSessionUpdateRuntime{Runtime: base, returned: returned}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions · Center")
	host.Type(target.Title)
	host.Shows(t, target.Title)

	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	host.Press(input.Esc)
	host.Shows(t, "updating favorite failed: session update")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCurrentSessionTimelineJumpsAndForksFromARootRun(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("create a timeline boundary")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	host.Shows(t, "complete")
	host.Type("/timeline")
	host.Press(input.Enter)
	host.Shows(t, "Current session timeline")
	host.Shows(t, "Run 1 of 1")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("timeline did not survive a temporarily minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("timeline viewport could not be restored")
	}
	host.Shows(t, "Current session timeline")
	host.Shows(t, "Run 1 of 1")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")

	host.Send(input.Key{Code: input.Character, Rune: ' ', Mods: 0})
	host.Type("/timeline")
	host.Press(input.Enter)
	host.Shows(t, "Current session timeline")
	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt})
	host.Shows(t, "session · Fork from")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionChangeOwnsTheComposerUntilItsSnapshotIsInstalled(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = stableCompletedScript
	backend := &blockingSessionChangeRuntime{
		Runtime:       base,
		changeStarted: make(chan struct{}),
		releaseChange: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: "/tmp/lyra-cli-test", StateDirectory: stateDirectory,
	})
	host.Shows(t, "Ask lyra")
	originalSession := firstRuntimeSession(t, base)
	host.Type("/new")
	host.Press(input.Enter)
	select {
	case <-backend.changeStarted:
	case <-time.After(time.Second):
		t.Fatal("session creation did not start")
	}

	host.Type("do not orphan this prompt")
	host.Press(input.Enter)
	host.Shows(t, "wait for the current session change")
	host.Until(t, "the rejected prompt to remain durable", func() bool {
		if !host.Repaint() {
			return false
		}
		store, err := workbench.Open(stateDirectory, workbench.Config{})
		if err != nil {
			return false
		}
		draft, found, readErr := store.Draft(originalSession)
		return readErr == nil && found && draft.Text == "do not orphan this prompt"
	})
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if history := store.History(); len(history) != 0 {
		t.Fatalf("rejected prompt was committed to history: %+v", history)
	}
	if calls := backend.starts.Load(); calls != 0 {
		t.Fatalf("prompt started %d run(s) before the session change settled", calls)
	}
	close(backend.releaseChange)
	host.Shows(t, "session · Untitled session")
	newSession := firstRuntimeSession(t, base)

	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	if calls := backend.starts.Load(); calls != 1 {
		t.Fatalf("prompt started %d runs after the session change", calls)
	}
	if got := backend.startedSession(); got != newSession {
		t.Fatalf("prompt started in %q, want newly installed session %q", got, newSession)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCtrlCCancelsSessionChangeWithoutDiscardingTheDraft(t *testing.T) {
	base := mock.New()
	base.Instant = true
	base.Script = stableCompletedScript
	backend := &blockingSessionChangeRuntime{
		Runtime:       base,
		changeStarted: make(chan struct{}),
		releaseChange: make(chan struct{}),
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	sourceSession := firstRuntimeSession(t, base)
	host.Type("/new")
	host.Press(input.Enter)
	select {
	case <-backend.changeStarted:
	case <-time.After(time.Second):
		t.Fatal("session creation did not start")
	}

	host.Type("keep this draft")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "session change canceled")
	host.Shows(t, "keep this draft")
	host.Press(input.Enter)
	host.Shows(t, "stable answer")
	if calls := backend.starts.Load(); calls != 1 {
		t.Fatalf("prompt started %d runs after canceling the session change", calls)
	}
	if got := backend.startedSession(); got != sourceSession {
		t.Fatalf("prompt started in %q, want original session %q", got, sourceSession)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestSessionChangeDoesNotInstallAfterAnInFlightDraftSaveFailure(t *testing.T) {
	base := mock.New()
	backend := &blockingSessionChangeRuntime{
		Runtime:       base,
		blockCreateAt: 1,
		changeStarted: make(chan struct{}),
		releaseChange: make(chan struct{}),
	}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft("ses_demo_1", agent.Message{Text: "saved before transition"}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, "", "ses_demo_1", stateDirectory)
	host.Shows(t, "saved before transition")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("start a new session")
	host.Shows(t, "/new")
	host.Press(input.Enter)
	select {
	case <-backend.changeStarted:
	case <-time.After(time.Second):
		t.Fatal("session creation did not start")
	}
	host.Shows(t, "creating session in /tmp/demo/store")

	draftsDirectory := filepath.Join(stateDirectory, "sessions")
	entries, err := os.ReadDir(draftsDirectory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("draft files = %d, %v", len(entries), err)
	}
	draftPath := filepath.Join(draftsDirectory, entries[0].Name())
	backupPath := draftPath + ".backup"
	if err := os.Rename(draftPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(draftPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "blocker"), []byte("block replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	host.Send(input.Paste{Text: " plus input during transition"})
	host.Shows(t, "saved before transition plus input during transition")
	host.Shows(t, "workbench:")
	close(backend.releaseChange)
	host.Shows(t, "failed: save source")
	host.Shows(t, "saved before transition plus input during transition")
	host.Shows(t, "Flaky cache expiry test")
	page, err := base.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 4 {
		t.Fatalf("runtime sessions = %d, want created session to remain discoverable", len(page.Items))
	}

	if err := os.Remove(filepath.Join(draftPath, "blocker")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(draftPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, draftPath); err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestSessionSwitchRebindsWorkspaceAttachmentsAndDropsOldChips(t *testing.T) {
	firstWorkspace := t.TempDir()
	secondWorkspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(firstWorkspace, "old.txt"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondWorkspace, "special.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := mock.New()
	backend.Instant = true
	if _, err := backend.CreateSession(t.Context(), agent.CreateSession{Title: "Workspace B", Workspace: secondWorkspace}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithWorkspace(t, backend, firstWorkspace)
	host.Shows(t, "Ask lyra")
	host.Type("@old")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached old.txt")

	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions")
	host.Type("Workspace B")
	host.Shows(t, "Workspace B")
	host.Press(input.Enter)
	host.Shows(t, "session · Workspace B")

	host.Type("/attachments")
	host.Press(input.Enter)
	host.Shows(t, "the composer has no attachments")
	host.Type("@special")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached special.txt")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func firstRuntimeSession(t *testing.T, runtime agent.SessionCatalog) string {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("latest session = %+v, %v", page, err)
	}
	return page.Items[0].ID
}

func TestProviderQualifiedModelAndLimitsApplyToTheNextRun(t *testing.T) {
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	configured := settings.Default()
	configured.Run.MaxSteps = 42
	configured.Run.MaxBudgetUSD = 2.5
	host, stop := runUIWithSettings(t, backend, configured)
	host.Shows(t, settings.DefaultProvider+"/"+settings.DefaultModel)

	host.Type("/model")
	host.Press(input.Enter)
	host.Shows(t, "Models")
	host.Type("Deep")
	host.Shows(t, "Synthetic Deep")
	host.Press(input.Enter)
	host.Shows(t, "model · synthetic/deep")

	host.Shows(t, "synthetic/deep")
	snapshot, err := backend.GetSession(t.Context(), firstRuntimeSession(t, backend))
	if err != nil || snapshot.Session.Model != "deep" {
		t.Fatalf("selected session model = %q, %v", snapshot.Session.Model, err)
	}

	host.Type("use these options")
	host.Press(input.Enter)
	host.Shows(t, "How should lyra proceed?")
	host.Press(input.Esc)
	host.Shows(t, "complete")
	if got := backend.options(); got.Provider != "synthetic" || got.Model != "deep" || got.Limits.MaxSteps != 42 || got.Limits.MaxBudgetUSD != 2.5 {
		t.Fatalf("StartRun options = %+v", got)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestRelocateMovesTheCurrentSessionAndRebindsWorkspaceState(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	source, destination := t.TempDir(), t.TempDir()
	host, stop := runUIWithWorkspace(t, backend, source)
	host.Shows(t, "Ask lyra")

	host.Type("/relocate " + destination)
	host.Press(input.Enter)
	want, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := firstRuntimeSession(t, backend)
	var snapshot agent.SessionSnapshot
	host.Until(t, "the current session to relocate", func() bool {
		var readErr error
		snapshot, readErr = backend.GetSession(t.Context(), sessionID)
		return readErr == nil && snapshot.Session.Workspace.Path == want && host.Repaint()
	})
	if snapshot.Session.Workspace.Path != want {
		t.Fatalf("relocated workspace = %q, want %q", snapshot.Session.Workspace.Path, want)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestRunRejectsAnUnresolvableAttachmentWorkspace(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "loop")
	if err := os.Symlink(workspace, workspace); err != nil {
		t.Fatal(err)
	}
	err := Run(t.Context(), Config{
		Runtime:   mock.New(),
		Workspace: workspace,
		Host:      programtest.New(t, 80, 24),
		Settings:  new(settings.Default()),
	})
	if err == nil || !strings.Contains(err.Error(), "session attachments") {
		t.Fatalf("Run error = %v, want attachment workspace failure", err)
	}
}

func TestPrepareSessionDistinguishesDefaultsFromExplicitFalseValues(t *testing.T) {
	configured := settings.Default()
	configured.UI.Mouse = false
	prepared, err := prepareSession(t.Context(), Config{
		Runtime: mock.New(), Workspace: t.TempDir(), Settings: new(configured),
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.settings.UI.Mouse {
		t.Fatal("explicit mouse=false was replaced by the default")
	}

	defaults, err := prepareSession(t.Context(), Config{Runtime: mock.New(), Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !defaults.settings.UI.Mouse || defaults.settings.Keys == nil {
		t.Fatalf("nil settings did not produce complete defaults: %+v", defaults.settings)
	}
}

func TestFormatThousandsHandlesMinimumInt64(t *testing.T) {
	if got := formatThousands(-1 << 63); got != "-9,223,372,036,854,775,808" {
		t.Fatalf("formatThousands(MinInt64) = %q", got)
	}
}

func TestQuestionFormSubmitsTypedAnswerAndCanCancel(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 2)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "question_1", Title: "Choose a strategy", Detail: "One short decision",
				Fields: []agent.QuestionField{{
					Header: "Strategy", Prompt: "Choose a strategy", Kind: agent.QuestionSingle,
					Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}},
				}},
			}},
			Continue: func(answerSet []agent.InterruptAnswer) []mock.Step {
				answers <- answerSet[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("ask me")
	host.Press(input.Enter)
	host.Shows(t, "Choose a strategy")
	host.Shows(t, "Safe")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	if answer := <-answers; len(answer.Values) != 1 || len(answer.Values[0]) != 1 || answer.Values[0][0] != "Safe" {
		t.Fatalf("submitted answer = %+v", answer)
	}

	host.Type("ask again")
	host.Press(input.Enter)
	host.Shows(t, "waiting for your answers")
	showsPlain(t, host, "╭─Choose a strategy")
	host.Press(input.Esc)
	host.Shows(t, "canceled")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestQuestionnaireSurvivesResizeBetweenFields(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "deployment-plan", Title: "Plan deployment", Detail: "Complete every field",
				Fields: []agent.QuestionField{
					{Header: "Goal", Prompt: "What should change?", Kind: agent.QuestionText},
					{Header: "Strategy", Prompt: "Choose an approach", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "Safe"}, {Label: "Fast"}}},
					{Header: "Checks", Prompt: "Select validation", Kind: agent.QuestionMulti, Options: []agent.QuestionOption{{Label: "Unit"}, {Label: "Integration"}}},
				},
			}},
			Continue: func(answerSet []agent.InterruptAnswer) []mock.Step {
				answers <- answerSet[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("prepare deployment")
	host.Press(input.Enter)
	host.Shows(t, "Plan deployment · 1/3")
	if !host.Resize(36, 10) {
		t.Fatal("resize while the first question field was open was refused")
	}
	host.Shows(t, "Goal — What should change?")
	host.Type("release safely")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("question field did not survive a temporarily minimal viewport")
	}
	if !host.Resize(36, 10) {
		t.Fatal("question field viewport could not be restored")
	}
	host.Shows(t, "release safely")
	host.Press(input.Enter)
	host.Shows(t, "Plan deployment · 2/3")
	host.Press(input.Esc)
	host.Shows(t, "Plan deployment · 1/3")
	host.Shows(t, "release safely")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("release carefully")
	host.Press(input.Enter)
	host.Shows(t, "Plan deployment · 2/3")
	if !host.Resize(120, 30) {
		t.Fatal("resize while the second question field was open was refused")
	}
	host.Press(input.Down)
	showsPlain(t, host, "● Fast")
	host.Press(input.Enter)
	host.Shows(t, "Plan deployment · 3/3")
	if !host.Resize(40, 12) {
		t.Fatal("resize while the third question field was open was refused")
	}
	host.Send(input.Key{Code: input.Character, Rune: ' '})
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: ' '})
	host.Press(input.Enter)
	host.Shows(t, "complete")

	answer := <-answers
	want := [][]string{{"release carefully"}, {"Fast"}, {"Unit", "Integration"}}
	if !slices.EqualFunc(answer.Values, want, slices.Equal) {
		t.Fatalf("submitted answer = %+v, want %+v", answer.Values, want)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCustomMultipleQuestionKeepsInvalidInputEditable(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "targets", Title: "Choose targets",
				Fields: []agent.QuestionField{{
					Prompt: "Targets", Kind: agent.QuestionMulti, AllowCustom: true,
					Options: []agent.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
				}},
			}},
			Continue: func(answerSet []agent.InterruptAnswer) []mock.Step {
				answers <- answerSet[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("choose targets")
	host.Press(input.Enter)
	host.Shows(t, "Choose targets")
	host.Shows(t, "Custom answer")
	host.Send(input.Key{Code: input.Character, Rune: ' '})
	host.Press(input.Down)
	host.Press(input.Down)
	host.Send(input.Key{Code: input.Character, Rune: ' '})
	host.Press(input.Tab)
	host.Type("linux")
	host.Press(input.Enter)
	host.Shows(t, `choice "linux" is duplicated`)
	select {
	case answer := <-answers:
		t.Fatalf("invalid custom choices reached the runtime: %+v", answer)
	default:
	}
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type("custom")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	want := []string{"linux", "custom"}
	if !slices.Equal(answer.Values[0], want) {
		t.Fatalf("custom choices = %q, want %q", answer.Values[0], want)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCustomSingleQuestionPreservesOptionsAndSurvivesResize(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.QuestionAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Question{
				ItemID: "platform", Title: "Choose platform", Detail: "Select a supported platform or provide another one",
				Fields: []agent.QuestionField{{
					Prompt: "Platform", Kind: agent.QuestionSingle, AllowCustom: true,
					Options: []agent.QuestionOption{
						{Label: "linux", Description: "Supported", Preview: "CI image"},
						{Label: "darwin", Description: "Experimental"},
					},
				}},
			}},
			Continue: func(answerSet []agent.InterruptAnswer) []mock.Step {
				answers <- answerSet[0].Answer.(agent.QuestionAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("choose a platform")
	host.Press(input.Enter)
	host.Shows(t, "linux — Supported · CI image")
	host.Press(input.Down)
	host.Press(input.Down)
	showsPlain(t, host, "● Other — provide a custom answer")
	host.Press(input.Enter)
	host.Shows(t, "an answer is required")
	host.Press(input.Tab)
	host.Type("freebsd")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("custom single-choice question did not survive a minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("custom single-choice question viewport could not be restored")
	}
	host.Shows(t, "freebsd")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if len(answer.Values) != 1 || !slices.Equal(answer.Values[0], []string{"freebsd"}) {
		t.Fatalf("custom single choice = %+v", answer.Values)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestWorkbenchRestoresHistoryAndSessionDraftAcrossLaunches(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = stableCompletedScript
	workspace, stateDirectory := t.TempDir(), t.TempDir()
	attachmentPath := filepath.Join(workspace, "context.txt")
	if err := os.WriteFile(attachmentPath, []byte("context"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, stopFirst := runUIWithState(t, backend, workspace, "", stateDirectory)
	first.Shows(t, "Ask lyra")
	first.Type("persisted history")
	first.Press(input.Enter)
	first.Shows(t, "stable answer")
	first.Type("/attach " + attachmentPath)
	first.Press(input.Enter)
	first.Shows(t, "attached context.txt")
	first.Type("unfinished draft")
	first.Shows(t, "unfinished draft")
	sessionID := firstRuntimeSession(t, backend)
	stopFirst()

	second, stopSecond := runUIWithState(t, backend, workspace, sessionID, stateDirectory)
	second.Shows(t, "unfinished draft")
	second.Shows(t, "@context.txt")
	second.Send(input.Key{Code: input.Up, Mods: input.Alt})
	second.Shows(t, "persisted history")
	second.Send(input.Key{Code: input.Down, Mods: input.Alt})
	second.Shows(t, "unfinished draft")
	second.Shows(t, "@context.txt")

	second.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stopSecond()
}

func TestUserCreatedSessionPreservesTheSourceDraft(t *testing.T) {
	backend := mock.New()
	stateDirectory := t.TempDir()
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "Ask lyra")
	host.Type("draft stays with the source session")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("start a new session")
	host.Shows(t, "/new")
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")
	host.Hides(t, "draft stays with the source session")
	replacementID := firstRuntimeSession(t, backend)
	stop()

	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	draft, found, err := store.Draft("ses_demo_1")
	if err != nil || !found || draft.Text != "draft stays with the source session" {
		t.Fatalf("source draft = %+v, found %t, error %v", draft, found, err)
	}
	if draft, found, err := store.Draft(replacementID); err != nil || found {
		t.Fatalf("new session draft = %+v, found %t, error %v", draft, found, err)
	}
}

func TestSessionChangeStopsBeforeMutationWhenTheSourceDraftCannotBeSaved(t *testing.T) {
	backend := mock.New()
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft("ses_demo_1", agent.Message{Text: "durable prefix"}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "durable prefix")

	draftsDirectory := filepath.Join(stateDirectory, "sessions")
	backupDirectory := filepath.Join(stateDirectory, "sessions-backup")
	if err := os.Rename(draftsDirectory, backupDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(draftsDirectory, []byte("block draft writes"), 0o600); err != nil {
		t.Fatal(err)
	}
	host.Send(input.Paste{Text: " plus unsaved input"})
	host.Shows(t, "workbench:")

	before, err := backend.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("start a new session")
	host.Shows(t, "/new")
	host.Press(input.Enter)
	host.Shows(t, "session change blocked: save session draft")
	host.Shows(t, "durable prefix plus unsaved input")
	after, err := backend.ListSessions(t.Context(), agent.SessionQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != len(before.Items) {
		t.Fatalf("failed draft save changed session count from %d to %d", len(before.Items), len(after.Items))
	}

	if err := os.Remove(draftsDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupDirectory, draftsDirectory); err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestPromptSubmissionStopsBeforeRuntimeWhenTheOutboxCannotBeSaved(t *testing.T) {
	base := mock.New()
	backend := &recordingRuntime{Runtime: base}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft("ses_demo_1", agent.Message{Text: "must remain editable"}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "must remain editable")

	draftsDirectory := filepath.Join(stateDirectory, "sessions")
	entries, err := os.ReadDir(draftsDirectory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("draft files = %d, %v", len(entries), err)
	}
	draftPath := filepath.Join(draftsDirectory, entries[0].Name())
	backupPath := draftPath + ".backup"
	if err := os.Rename(draftPath, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(draftPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draftPath, "blocker"), []byte("block deletion"), 0o600); err != nil {
		t.Fatal(err)
	}

	host.Press(input.Enter)
	host.Shows(t, "prompt submission blocked: save pending run")
	host.Shows(t, "must remain editable")
	if got := backend.startCount(); got != 0 {
		t.Fatalf("runtime started %d runs after draft retirement failed", got)
	}

	if err := os.Remove(filepath.Join(draftPath, "blocker")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(draftPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backupPath, draftPath); err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestPromptSubmissionCommitsHistoryOnlyAfterRuntimeAcknowledgement(t *testing.T) {
	base := mock.New()
	backend := &recordingRuntime{Runtime: base}
	stateDirectory := t.TempDir()
	store, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft("ses_demo_1", agent.Message{Text: "history must commit first"}); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithState(t, backend, "/tmp/lyra-cli-test", "ses_demo_1", stateDirectory)
	host.Shows(t, "history must commit first")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	if got := backend.startCount(); got != 1 {
		t.Fatalf("runtime started %d runs, want one", got)
	}
	reopened, err := workbench.Open(stateDirectory, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if history := reopened.History(); len(history) != 1 || history[0].Text != "history must commit first" {
		t.Fatalf("history after runtime acknowledgement = %+v", history)
	}
	if draft, found, err := reopened.Draft("ses_demo_1"); err != nil || found {
		t.Fatalf("draft after runtime acknowledgement = %+v, found %t, error %v", draft, found, err)
	}
	stop()
}

func TestPromptStashCanBeListedAppliedAndDeleted(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("draft to preserve")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("stash current prompt")
	host.Shows(t, "/stash")
	host.Press(input.Enter)
	host.Shows(t, "stashed prompt")
	match := regexp.MustCompile(`stashed prompt · ([0-9a-f]{16})`).FindStringSubmatch(host.Frame())
	if len(match) != 2 {
		t.Fatalf("stash id is not visible:\n%s", host.Frame())
	}
	prefix := match[1][:8]
	host.Type("/stashes")
	host.Press(input.Enter)
	host.Shows(t, "draft to preserve")
	host.Type("/stash-apply " + prefix)
	host.Press(input.Enter)
	host.Shows(t, "applied prompt stash")
	host.Shows(t, "draft to preserve")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Type("/stash-delete " + prefix)
	host.Press(input.Enter)
	host.Shows(t, "deleted prompt stash")
	host.Type("/stashes")
	host.Press(input.Enter)
	host.Shows(t, "there are no prompt stashes")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestExternalEditorRoundTripReplacesOnlyAfterSuccess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Setenv("LYRA_EDITOR", `sh -c 'printf "\nrevised externally" >> "$0"'`)
		host, stop := runUIWithWorkspace(t, mock.New(), t.TempDir())
		host.Shows(t, "Ask lyra")
		host.Type("original draft")
		host.Send(input.Key{Code: input.Character, Rune: 'e', Mods: input.Ctrl})
		host.Shows(t, "updated prompt from external editor")
		host.Shows(t, "original draft")
		host.Shows(t, "revised externally")
		host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
		stop()
	})

	t.Run("failure preserves draft", func(t *testing.T) {
		t.Setenv("LYRA_EDITOR", `sh -c 'exit 9'`)
		host, stop := runUIWithWorkspace(t, mock.New(), t.TempDir())
		host.Shows(t, "Ask lyra")
		host.Type("do not lose this")
		host.Send(input.Key{Code: input.Character, Rune: 'e', Mods: input.Ctrl})
		host.Shows(t, "exit status 9")
		host.Shows(t, "do not lose this")
		host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
		stop()
	})
}

func TestApprovalDenialSubmitsOptionalUserFeedback(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan []agent.InterruptAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval-feedback", Title: "Run destructive command", Detail: "Review this request carefully",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "rm generated.txt", Status: agent.ToolRunning,
					ArgumentsJSON: []byte(`{"command":"rm generated.txt","generatedFixture":"cache_test.go"}`),
				},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("remove generated output")
	host.Press(input.Enter)
	host.Shows(t, "$ rm generated.txt")
	host.Shows(t, "generatedFixture")
	host.Shows(t, "cache_test.go")
	host.Press(input.Down)
	host.Press(input.Tab)
	host.Type("keep the generated fixture")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	provided := <-answers
	answer, ok := provided[0].Answer.(agent.ApprovalAnswer)
	if !ok || answer.Decision != agent.ApprovalDeny || answer.Reason != "keep the generated fixture" {
		t.Fatalf("denial answer = %#v", provided[0].Answer)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalCanRememberADenialWithoutLosingFeedback(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "remember-denial", Title: "Delete generated fixtures", Rememberable: true,
				RuleHint: "shell:rm generated/*",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "rm generated/*", Status: agent.ToolRunning,
				},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("protect generated fixtures")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	for range 5 {
		host.Press(input.Down)
	}
	showsPlain(t, host, "● Deny for this session")
	host.Press(input.Tab)
	host.Type("preserve generated fixtures")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if answer.Decision != agent.ApprovalDeny || answer.Remember != agent.RememberSession ||
		answer.Reason != "preserve generated fixtures" || answer.ArgumentOverride != nil {
		t.Fatalf("remembered denial = %+v", answer)
	}
	rules, err := backend.ListApprovalRules(t.Context(), firstRuntimeSession(t, backend))
	if err != nil || len(rules) != 1 || rules[0].Decision != agent.ApprovalRuleDeny ||
		rules[0].Scope != agent.RememberSession {
		t.Fatalf("remembered denial rules = %+v, %v", rules, err)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalCanEditToolArgumentsOnceAcrossValidationAndResize(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "edit-approval", Title: "Run generated command",
				Rememberable: true,
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "rm generated.txt", Status: agent.ToolRunning,
					ArgumentsJSON: []byte(`{"command":"rm generated.txt","timeout":30}`),
				},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("run a safer generated command")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.End)
	showsPlain(t, host, "● Edit arguments before deciding")
	host.Press(input.Enter)
	host.Shows(t, "Edit tool arguments")
	host.Shows(t, `"command": "rm generated.txt"`)

	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type(`{"command":"echo safe","timeout":`)
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "tool argument override")
	host.Shows(t, `{"command":"echo safe","timeout":`)
	select {
	case premature := <-answers:
		t.Fatalf("invalid argument override resumed runtime: %+v", premature)
	default:
	}

	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type(`{"command":"echo safe","timeout":45}`)
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("argument editor did not survive a minimal viewport")
	}
	host.Shows(t, `{"command":"echo safe","timeout":45}`)
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Shows(t, "Edited arguments · one-shot")
	showsPlain(t, host, `"command": "echo safe"`)
	showsPlain(t, host, "● Allow once")
	host.Press(input.Down)
	showsPlain(t, host, "● Allow for this session")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if answer.Decision != agent.ApprovalApprove || answer.Remember != agent.RememberSession ||
		answer.ArgumentOverride == nil ||
		string(answer.ArgumentOverride.JSON()) != `{"command":"echo safe","timeout":45}` {
		t.Fatalf("edited approval answer = %+v", answer)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelingApprovalArgumentEditReturnsToTheUnchangedApproval(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "cancel-edit-approval", Title: "Run generated command",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Command: "echo original", Status: agent.ToolRunning,
					ArgumentsJSON: []byte(`{"command":"echo original"}`),
				},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("cancel an argument edit")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.End)
	host.Press(input.Enter)
	host.Shows(t, "Edit tool arguments")
	host.Send(input.Key{Code: input.Character, Rune: 'a', Mods: input.Alt})
	host.Type(`{"command":"echo abandoned"}`)
	host.Press(input.Esc)
	host.Hides(t, "Edit tool arguments")
	host.Shows(t, "Tool approval")
	host.Hides(t, "Edited arguments · one-shot")
	host.Press(input.Home)
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if answer.Decision != agent.ApprovalApprove || answer.ArgumentOverride != nil {
		t.Fatalf("canceled argument edit leaked into approval answer: %+v", answer)
	}
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalStateSurvivesMinimalViewportAndRestores(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "resize-approval", Title: "Run generated command",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "rm generated.txt", Status: agent.ToolRunning},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("review command")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Down)
	host.Press(input.Tab)
	host.Type("KEEP_RESIZE_FEEDBACK")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("approval did not survive a temporarily minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("approval viewport could not be restored")
	}
	host.Shows(t, "KEEP_RESIZE_FEEDBACK")
	showsPlain(t, host, "● Deny")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if answer.Decision != agent.ApprovalDeny || answer.Reason != "KEEP_RESIZE_FEEDBACK" {
		t.Fatalf("restored approval answer = %+v", answer)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestNonRememberableApprovalOverridesConfiguredRememberDefault(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan agent.ApprovalAnswer, 1)
	backend.Script = func(string) mock.Script {
		return mock.Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "one-shot-approval", Title: "Read generated report",
				Tool: &agent.ToolCall{Kind: agent.ToolRead, Name: "read", Path: "report.txt", Status: agent.ToolRunning},
			}},
			Continue: func(provided []agent.InterruptAnswer) []mock.Step {
				answers <- provided[0].Answer.(agent.ApprovalAnswer)
				return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	configured := settings.Default()
	configured.Approval.Remember = settings.RememberProject
	host, stop := runUIWithSettings(t, backend, configured)
	host.Shows(t, "Ask lyra")
	host.Type("read report")
	host.Press(input.Enter)
	showsPlain(t, host, "● Allow once")
	host.Hides(t, "Allow for this project")
	host.Hides(t, "Always allow this rule")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	answer := <-answers
	if answer.Decision != agent.ApprovalApprove || answer.Remember != agent.RememberNone {
		t.Fatalf("one-shot approval answer = %+v", answer)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestMultiInteractionReviewSupportsBackEditAndOneFinalResume(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan []agent.InterruptAnswer, 2)
	backend.Script = func(string) mock.Script { return multiInteractionReviewScript(answers) }
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("test the project")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	showsPlain(t, host, "● Allow once")
	host.Press(input.Enter)
	host.Shows(t, "Choose platform")
	host.Press(input.Esc)
	host.Shows(t, "$ go test ./...")
	showsPlain(t, host, "● Allow once")
	host.Press(input.Enter)
	host.Shows(t, "Choose platform")
	showsPlain(t, host, "● Linux")
	host.Press(input.Enter)
	host.Shows(t, "Review interactions")
	if !host.Resize(1, 1) || !host.Repaint() {
		t.Fatal("interaction review did not survive a temporarily minimal viewport")
	}
	if !host.Resize(96, 28) {
		t.Fatal("interaction review viewport could not be restored")
	}
	host.Shows(t, "Review interactions")
	select {
	case premature := <-answers:
		t.Fatalf("runtime resumed before final review: %+v", premature)
	default:
	}
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Hides(t, "Review interactions")
	host.Shows(t, "Choose platform")
	host.Press(input.Down)
	showsPlain(t, host, "● Darwin")
	host.Press(input.Enter)
	host.Shows(t, "Review interactions")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	provided := <-answers
	if len(provided) != 2 {
		t.Fatalf("interaction answers = %+v", provided)
	}
	question, ok := provided[1].Answer.(agent.QuestionAnswer)
	if !ok || question.Values[0][0] != "Darwin" {
		t.Fatalf("edited question answer = %#v", provided[1].Answer)
	}
	select {
	case duplicate := <-answers:
		t.Fatalf("runtime resumed twice: %+v", duplicate)
	default:
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelingInteractionReviewDoesNotResumeTheRuntime(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	answers := make(chan []agent.InterruptAnswer, 1)
	backend.Script = func(string) mock.Script { return multiInteractionReviewScript(answers) }
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("review then cancel")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	showsPlain(t, host, "● Allow once")
	host.Press(input.Enter)
	host.Shows(t, "Choose platform")
	showsPlain(t, host, "● Linux")
	host.Press(input.Enter)
	host.Shows(t, "Review interactions")
	host.Press(input.Down)
	host.Press(input.Down)
	showsPlain(t, host, "● Cancel the run")
	host.Press(input.Enter)
	host.Shows(t, "canceled")
	select {
	case provided := <-answers:
		t.Fatalf("canceled review resumed the runtime: %+v", provided)
	default:
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func multiInteractionReviewScript(answers chan<- []agent.InterruptAnswer) mock.Script {
	return mock.Script{
		Interactions: []agent.Interaction{
			agent.Approval{
				ItemID: "approval", Title: "Run tests", Rememberable: true,
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
			},
			agent.Question{
				ItemID: "question", Title: "Choose platform",
				Fields: []agent.QuestionField{{Prompt: "Platform", Kind: agent.QuestionSingle, Options: []agent.QuestionOption{{Label: "Linux"}, {Label: "Darwin"}}}},
			},
		},
		Continue: func(provided []agent.InterruptAnswer) []mock.Step {
			answers <- provided
			return []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
		},
	}
}

func TestApprovalRememberScopeAppliesToLaterRuns(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("first edit")
	host.Press(input.Enter)
	host.Shows(t, "How should lyra proceed?")
	host.Press(input.Down)
	host.Press(input.Enter)
	host.Shows(t, "complete")

	host.Type("second edit")
	host.Press(input.Enter)
	host.Shows(t, "Applied remembered approval rule")
	host.Shows(t, "complete")
	host.Hides(t, "How should lyra proceed?")
	sessionID := firstRuntimeSession(t, backend)
	rules, err := backend.ListApprovalRules(t.Context(), sessionID)
	if err != nil || len(rules) != 1 || rules[0].Scope != agent.RememberSession {
		t.Fatalf("remembered rules = %+v, %v", rules, err)
	}
	host.Type("/rules")
	host.Press(input.Enter)
	host.Shows(t, "approval rules")
	host.Shows(t, rules[0].ID)
	host.Shows(t, "session  allow")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalModeSelectionRoundTripsThroughTheRuntime(t *testing.T) {
	backend := mock.New()
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("/approval")
	host.Press(input.Enter)
	host.Shows(t, "Runtime approval mode")
	host.Press(input.Enter)
	host.Shows(t, "approval mode · safe")
	mode, err := backend.GetApprovalMode(t.Context())
	if err != nil || mode != agent.ApprovalModeSafe {
		t.Fatalf("approval mode = (%q, %v)", mode, err)
	}

	host.Type("/rules")
	host.Press(input.Enter)
	host.Shows(t, "no remembered approval rules")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCommandPaletteSearchAndDetailShortcutsAreReachable(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("status")
	host.Shows(t, "/status")
	host.Press(input.Enter)
	host.Shows(t, "runtime options")

	host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Ctrl})
	host.Shows(t, "Find in the live transcript")
	host.Type("model")
	host.Press(input.Enter)
	host.Shows(t, "match(es) for")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "tool details expanded")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "tool details collapsed")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestExactSlashCommandRunsWithOneEnterWhilePartialInputCompletes(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("/help")
	host.Press(input.Enter)
	host.Shows(t, "Commands")
	host.Press(input.Esc)
	host.Hides(t, "Commands")

	host.Type("/sho")
	host.Press(input.Enter)
	host.Shows(t, "/shortcuts")
	host.Hides(t, "Shortcuts")
	host.Press(input.Enter)
	host.Shows(t, "Shortcuts")
	host.Press(input.Esc)
	host.Hides(t, "Shortcuts")

	host.Type("/resume")
	host.Press(input.Enter)
	host.Shows(t, "Sessions")
	host.Press(input.Esc)

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCommandPaletteSharesContextAvailabilityWithExecution(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Type("view")
	host.Shows(t, "Transcript · unavailable: select a readable transcript entry first")
	host.Press(input.Enter)
	host.Shows(t, "/view unavailable: select a readable transcript entry first")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestPendingConfiguredChordShowsItsContinuationsUntilResolved(t *testing.T) {
	configured := settings.Default()
	configured.Keys[settings.ActionCommandPalette] = []string{"ctrl+k ctrl+p"}
	host, stop := runUIWithSettings(t, mock.New(), configured)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'k', Mods: input.Ctrl})
	host.Shows(t, "ctrl+k → ctrl+p command-palette")
	host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl})
	host.Shows(t, "Commands")
	host.Hides(t, "ctrl+k →")

	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestShortcutGuideReflectsActiveBindings(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Send(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	host.Shows(t, "Shortcuts")
	host.Shows(t, "ctrl+x")
	host.Shows(t, "open this shortcut guide")
	host.Shows(t, "shift+enter / alt+enter")
	host.Shows(t, "insert a newline")
	host.Press(input.End)
	host.Hides(t, "scroll this guide up")
	host.Press(input.Esc)
	host.Hides(t, "open this shortcut guide")

	host.Type("/shortcuts")
	host.Press(input.Enter)
	host.Shows(t, "Shortcuts")
	host.Shows(t, "scroll this guide up")
	host.Press(input.Esc)
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestWorkspaceFileCompletionCreatesAtomicAttachments(t *testing.T) {
	workspace := t.TempDir()
	path := workspace + "/cache_test.go"
	if err := os.WriteFile(path, []byte("package cache\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("@cache")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attached cache_test.go")

	// Commands operate on, but do not accidentally submit, staged attachments.
	host.Type("/attachments")
	host.Press(input.Enter)
	host.Shows(t, "attachments")
	host.Shows(t, "cache_test.go · text/")
	host.Type("/detach all")
	host.Press(input.Enter)
	host.Shows(t, "removed all attachments")

	host.Type("/attach cache_test.go")
	host.Press(input.Enter)
	host.Shows(t, "attached cache_test.go")
	host.Type("inspect this file")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	started := backend.startInput()
	if started.Message.Text != "inspect this file" || len(started.Message.Attachments) != 1 {
		t.Fatalf("start message = %+v", started.Message)
	}
	canonical, _ := filepath.EvalSymlinks(path)
	if got := started.Message.Attachments[0]; got.Path != canonical || got.Kind != agent.AttachmentText {
		t.Fatalf("attachment = %+v", got)
	}

	// Semantic prompt history restores the chip, not just its visible @label.
	host.Send(input.Key{Code: input.Up, Mods: input.Alt})
	host.Shows(t, "@cache_test.go")
	host.Shows(t, "inspect this file")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestRejectedWorkspaceFileCompletionPreservesTheDraftToken(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "archive.bin"), []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	host, stop := runUIWithWorkspace(t, mock.New(), workspace)
	host.Shows(t, "Ask lyra")
	host.Type("inspect @archive")
	host.Shows(t, "workspace files")
	host.Press(input.Enter)
	host.Shows(t, "attachment must be text or an image")
	host.Hides(t, "workspace files")
	host.Shows(t, "inspect @archive")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestUndoAfterDetachRestoresTheAttachmentValue(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "context.txt")
	if err := os.WriteFile(path, []byte("restorable context"), 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	host.Type("/attach context.txt")
	host.Press(input.Enter)
	host.Shows(t, "attached context.txt")
	host.Type("/detach all")
	host.Press(input.Enter)
	host.Shows(t, "removed all attachments")

	host.Send(input.Key{Code: input.Character, Rune: '_', Mods: input.Ctrl})
	host.Shows(t, "@context.txt")
	host.Type("use restored context")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	started := backend.startInput()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if started.Message.Text != "use restored context" || len(started.Message.Attachments) != 1 || started.Message.Attachments[0].Path != canonical {
		t.Fatalf("restored attachment input = %+v", started.Message)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestDetachRejectsAnAmbiguousAttachmentBasename(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"one/shared.txt", "two/shared.txt"} {
		path := filepath.Join(workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	backend := &recordingRuntime{Runtime: mock.New()}
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	host, stop := runUIWithWorkspace(t, backend, workspace)
	host.Shows(t, "Ask lyra")
	for _, name := range []string{"one/shared.txt", "two/shared.txt"} {
		host.Type("/attach " + name)
		host.Press(input.Enter)
		host.Shows(t, "attached "+name)
	}
	host.Type("/detach shared.txt")
	host.Press(input.Enter)
	host.Shows(t, `attachment "shared.txt" is ambiguous`)
	host.Type("/detach 2")
	host.Press(input.Enter)
	host.Shows(t, "detached two/shared.txt")
	host.Type("use the unambiguous attachment")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	started := backend.startInput()
	if len(started.Message.Attachments) != 1 || started.Message.Attachments[0].Name != "one/shared.txt" {
		t.Fatalf("attachments after detach = %+v", started.Message.Attachments)
	}

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestToolKindsRenderLiveAndDetailToggleChangesTheTranscript(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	backend.Script = func(string) mock.Script {
		zero := 0
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: agent.ToolRunning,
			}}}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "SHELL_DETAIL_"}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "OK\nsecond line"}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "provider.exec", Command: "go test ./...", Summary: "run tests", Status: agent.ToolOK,
				Output: "SHELL_DETAIL_OK\nsecond line", ExitCode: &zero,
			}}}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "edit", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolEdit, Name: "provider.patch", Path: "internal/cache.go", Summary: "update cache", Status: agent.ToolOK,
				Diff: "--- a/internal/cache.go\n+++ b/internal/cache.go\n@@ -1,2 +1,2 @@\n package cache\n-oldTicker()\n+newSweepSignal()\n",
			}}}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "search", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolSearch, Name: "provider.grep", Query: "cache expiry", Summary: "find expiry", Status: agent.ToolOK,
				Output: "internal/cache.go:22\ninternal/cache_test.go:18",
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		},
		}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("show tool views")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	host.Shows(t, "edit · internal/cache.go")
	host.Shows(t, "search · cache expiry")
	host.Hides(t, "SHELL_DETAIL_OK")
	host.Hides(t, "newSweepSignal")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "SHELL_DETAIL_OK")
	host.Shows(t, "newSweepSignal")
	host.Shows(t, "internal/cache_test.go:18")

	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Hides(t, "SHELL_DETAIL_OK")
	host.Hides(t, "newSweepSignal")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestRunningToolOutputStreamsIntoAnExpandedTranscript(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		zero := 0
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolRunning,
			}}}},
			{Delay: 100 * time.Millisecond, Event: agent.BlockDelta{BlockID: "shell", Text: "LIVE_TOOL_FIRST\n"}},
			{Delay: 400 * time.Millisecond, Event: agent.BlockDelta{BlockID: "shell", Text: "LIVE_TOOL_SECOND\n"}},
			{Delay: 400 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "go test ./...", Status: agent.ToolOK,
				Output: "LIVE_TOOL_FIRST\nLIVE_TOOL_SECOND\n", ExitCode: &zero,
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("stream tool output")
	host.Press(input.Enter)
	host.Shows(t, "$ go test ./...")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "LIVE_TOOL_FIRST")
	host.Shows(t, "LIVE_TOOL_SECOND")
	host.Shows(t, "done")
	host.Shows(t, "complete")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelingARunSettlesItsLiveToolProjection(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "long command", Status: agent.ToolRunning,
			}}}},
			{Event: agent.BlockDelta{BlockID: "shell", Text: "PARTIAL_TOOL_OUTPUT\n"}},
			{Delay: time.Hour, Event: agent.BlockCompleted{Block: agent.Block{ID: "shell", Kind: agent.BlockTool, Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Command: "long command", Status: agent.ToolOK, Output: "never reached",
			}}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	host, stop := runUIWith(t, backend)
	host.Shows(t, "Ask lyra")
	host.Type("cancel live tool")
	host.Press(input.Enter)
	host.Shows(t, "$ long command")
	host.Send(input.Key{Code: input.Character, Rune: 'o', Mods: input.Ctrl})
	host.Shows(t, "PARTIAL_TOOL_OUTPUT")
	host.Press(input.Esc)
	host.Shows(t, "canceled")
	host.Hides(t, "running")
	host.Hides(t, "never reached")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCancelBeforeRunIdentityDoesNotBlockTheNextRun(t *testing.T) {
	backend := mock.New()
	backend.Instant = true
	runtime := &delayedFirstRuntime{Runtime: backend}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("first request waits before returning a stream")
	host.Press(input.Enter)
	host.Until(t, "the first runtime handshake to block", func() bool {
		return runtime.starts.Load() == 1 && host.Repaint()
	})
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Shows(t, "canceled")

	host.Type("second request can start")
	host.Press(input.Enter)
	host.Shows(t, "Tool approval")
	host.Press(input.Esc)
	host.Shows(t, "left unchanged")
	host.Shows(t, "complete")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestApprovalRemainsUsableAtRepresentativeWidths(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{
		{name: "narrow", width: 44, height: 18},
		{name: "wide", width: 120, height: 32},
	} {
		t.Run(size.name, func(t *testing.T) {
			backend := mock.New()
			backend.Instant = true
			backend.Script = approvalWidthScript
			host, stop := runUIWith(t, backend)
			host.Shows(t, "Ask lyra")
			host.Type("review this at the current terminal width")
			host.Press(input.Enter)
			host.Shows(t, "How should lyra proceed?")
			if !host.Resize(size.width, size.height) {
				t.Fatalf("resize open approval to %dx%d was refused", size.width, size.height)
			}
			host.Shows(t, "How should lyra proceed?")
			host.Press(input.Esc)
			host.Shows(t, "Approval denied at current width")
			host.Shows(t, "complete")

			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func approvalWidthScript(string) mock.Script {
	return mock.Script{
		Interactions: []agent.Interaction{agent.Approval{
			ItemID: "responsive-approval",
			Title:  "Review the proposed change",
			Detail: "Confirm that the approval form remains usable at this terminal width.",
			Tool: &agent.ToolCall{
				Kind: agent.ToolShell, Name: "responsive-check", Command: "go test ./...", Status: agent.ToolRunning,
			},
		}},
		Continue: func(answers []agent.InterruptAnswer) []mock.Step {
			message := "Approval was not denied at current width"
			if len(answers) == 1 {
				answer, ok := answers[0].Answer.(agent.ApprovalAnswer)
				if ok && answer.Decision == agent.ApprovalDeny {
					message = "Approval denied at current width"
				}
			}
			return []mock.Step{
				{Event: agent.BlockCompleted{Block: agent.Block{ID: "responsive-result", Kind: agent.BlockAssistant, Text: message}}},
				{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
			}
		},
	}
}

func TestTerminalSurvivesExtremeResizeAndRemainsInteractive(t *testing.T) {
	host, stop := runUI(t)
	host.Shows(t, "Ask lyra")
	host.Type("resize draft")
	for _, size := range []struct{ width, height int }{{20, 7}, {10, 7}, {200, 60}, {32, 10}, {96, 28}} {
		if !host.Resize(size.width, size.height) {
			t.Fatalf("resize to %dx%d was refused", size.width, size.height)
		}
		if !host.Repaint() {
			t.Fatalf("repaint at %dx%d was refused", size.width, size.height)
		}
	}
	host.Shows(t, "resize draft")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	host.Hides(t, "resize draft")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Shows(t, "terminal.core@1.0.0")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	stop()
}

func TestCatalogDialogsRestoreAfterMinimalViewport(t *testing.T) {
	for _, test := range []struct {
		command string
		title   string
	}{
		{command: "/help", title: "Commands"},
		{command: "/shortcuts", title: "Shortcuts"},
		{command: "/sessions", title: "Sessions"},
		{command: "/workspace", title: "Workspaces"},
		{command: "/model", title: "Models"},
		{command: "/approval", title: "Runtime approval mode"},
	} {
		t.Run(strings.TrimPrefix(test.command, "/"), func(t *testing.T) {
			host, stop := runUI(t)
			host.Shows(t, "Ask lyra")
			host.Type(test.command)
			host.Press(input.Enter)
			host.Shows(t, test.title)
			if !host.Resize(1, 1) || !host.Repaint() {
				t.Fatalf("%s did not survive a temporarily minimal viewport", test.title)
			}
			if !host.Resize(96, 28) {
				t.Fatalf("%s viewport could not be restored", test.title)
			}
			host.Shows(t, test.title)
			host.Press(input.Esc)
			host.Shows(t, "Ask lyra")
			host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
			stop()
		})
	}
}

func TestStreamingRemainsResponsiveThroughAResizeStorm(t *testing.T) {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		steps := []mock.Step{{Event: agent.BlockStarted{Block: agent.Block{
			ID: "stream", Kind: agent.BlockAssistant,
		}}}}
		for range 64 {
			steps = append(steps, mock.Step{
				Delay: 2 * time.Millisecond,
				Event: agent.BlockDelta{BlockID: "stream", Text: "x"},
			})
		}
		steps = append(steps,
			mock.Step{Event: agent.BlockCompleted{Block: agent.Block{
				ID: "stream", Kind: agent.BlockAssistant, Text: "RESIZE_STREAM_COMPLETE",
			}}},
			mock.Step{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		)
		return mock.Script{Prelude: steps}
	}
	host, stop := runUIWith(t, runtime)
	host.Shows(t, "Ask lyra")
	host.Type("stress the viewport")
	host.Press(input.Enter)

	sizes := []struct{ width, height int }{
		{1, 1}, {120, 40}, {8, 3}, {96, 28}, {11, 20}, {200, 60}, {20, 7}, {80, 24},
	}
	for iteration := range 8 {
		for _, size := range sizes {
			if !host.Resize(size.width, size.height) || !host.Repaint() {
				t.Fatalf("resize storm stopped at iteration %d size %dx%d", iteration, size.width, size.height)
			}
		}
	}
	if !host.Resize(96, 28) {
		t.Fatal("could not restore the normal viewport")
	}
	host.Shows(t, "RESIZE_STREAM_COMPLETE")
	host.Shows(t, "complete")
	host.Type("/plugins")
	host.Press(input.Enter)
	host.Shows(t, "terminal.core")
	stop()
}

func TestOpeningAnActiveSessionRecoversAStreamWhoseTransientStartPredatesAttachment(t *testing.T) {
	backend := mock.New()
	backend.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}}},
			{Event: agent.BlockDelta{BlockID: "answer", Text: "provisional"}},
			{Delay: 200 * time.Millisecond, Event: agent.BlockDelta{BlockID: "answer", Text: " preview"}},
			{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "RECOVERED_AUTHORITATIVE_ANSWER"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, err := backend.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := backend.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "recover me"}})
	if err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, delta := event.Event.(agent.BlockDelta); delta {
			break
		}
	}

	host, stop := runUIForSession(t, backend, session.ID)
	host.Shows(t, "RECOVERED_AUTHORITATIVE_ANSWER")
	host.Shows(t, "complete")
	stop()
}

func TestOutcomeNotificationMatchesTheRunVerdict(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome agent.Outcome
		want    string
	}{
		{name: "completed", outcome: agent.Outcome{Status: agent.OutcomeCompleted}, want: "lyra run completed"},
		{name: "canceled", outcome: agent.Outcome{Status: agent.OutcomeCanceled}, want: "lyra run canceled"},
		{name: "failed", outcome: agent.Outcome{Status: agent.OutcomeFailed, Problem: &failure.Problem{Type: "provider_error", Detail: "boom"}}, want: "lyra run failed"},
		{name: "unsettled", outcome: agent.Outcome{}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := outcomeNotification(test.outcome); got != test.want {
				t.Fatalf("outcomeNotification(%+v) = %q, want %q", test.outcome, got, test.want)
			}
		})
	}
}
