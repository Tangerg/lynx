package terminal

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/codebase"
	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
	"github.com/Tangerg/lynx/app/cli/internal/hookpolicy"
)

type diagnosticToolServiceStub struct {
	invoked chan diagnostictool.Invocation
}

func (service *diagnosticToolServiceStub) Tools(context.Context) ([]diagnostictool.Descriptor, error) {
	return []diagnostictool.Descriptor{{
		Name: "inspect.cache", Description: "inspect cache ownership", Safety: diagnostictool.Safe,
		Schema: json.RawMessage(`{"type":"object","properties":{"depth":{"type":"number"}}}`),
	}}, nil
}

func (service *diagnosticToolServiceStub) Invoke(_ context.Context, invocation diagnostictool.Invocation) (diagnostictool.Result, error) {
	service.invoked <- invocation
	return diagnostictool.Result{JSON: json.RawMessage(`{"entries":2,"healthy":true}`)}, nil
}

func TestDiagnosticToolsRenderSchemaAndConfinedResultAcrossResize(t *testing.T) {
	tools := &diagnosticToolServiceStub{invoked: make(chan diagnostictool.Invocation, 1)}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Workspace: "/workspace", DiagnosticTools: tools})
	host.Shows(t, "Ask lyra")
	host.Type("/tools")
	host.Press(input.Enter)
	host.Shows(t, "Diagnostic tools")
	host.Shows(t, "inspect.cache")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type(`/tool-invoke inspect {"depth":2}`)
	host.Press(input.Enter)
	invocation := awaitValue(t, tools.invoked, "diagnostic invocation")
	if invocation.Tool.Name != "inspect.cache" || invocation.Workspace != "/workspace" || string(invocation.Arguments) != `{"depth":2}` {
		t.Fatalf("invocation = %+v", invocation)
	}
	host.Shows(t, "Diagnostic · inspect.cache")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("diagnostic result did not survive a minimal viewport")
	}
	showsPlain(t, host, `"healthy": true`)
	stop()
}

type codebaseServiceStub struct {
	reindexed chan string
}

type blockingCodebaseReindexService struct {
	codebase.Service
	started  chan string
	release  chan struct{}
	canceled chan struct{}
}

func (service *blockingCodebaseReindexService) Reindex(
	ctx context.Context,
	workspace string,
) (codebase.ReindexOperation, error) {
	service.started <- workspace
	select {
	case <-service.release:
		return service.Service.Reindex(ctx, workspace)
	case <-ctx.Done():
		close(service.canceled)
		return codebase.ReindexOperation{}, context.Cause(ctx)
	}
}

func (*codebaseServiceStub) Status(context.Context, string) (codebase.Status, error) {
	now := time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC)
	return codebase.Status{State: codebase.Ready, ModelID: "embed/model", FileCount: 12, ChunkCount: 24, IndexedAt: &now}, nil
}

func (*codebaseServiceStub) Search(_ context.Context, query codebase.Query) ([]codebase.Hit, error) {
	return []codebase.Hit{{Path: "internal/owner.go", StartLine: 10, EndLine: 14, Snippet: "type Owner struct{}", Score: .875}}, nil
}

func (service *codebaseServiceStub) Reindex(_ context.Context, workspace string) (codebase.ReindexOperation, error) {
	service.reindexed <- workspace
	return codebase.ReindexOperation{ID: "op_reindex"}, nil
}

func TestCodebaseStatusSearchAndResizeSafeReindexConfirmation(t *testing.T) {
	index := &codebaseServiceStub{reindexed: make(chan string, 1)}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Workspace: "/workspace", Codebase: index})
	host.Shows(t, "Ask lyra")
	host.Type("/codebase")
	host.Press(input.Enter)
	host.Shows(t, "Codebase index")
	host.Shows(t, "files      12")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/codebase-search ownership")
	host.Press(input.Enter)
	host.Shows(t, "internal/owner.go:10-14")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/codebase-reindex")
	host.Press(input.Enter)
	host.Shows(t, "Reindex codebase")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("codebase confirmation did not survive a minimal viewport")
	}
	host.Shows(t, "Reindex codebase")
	host.Press(input.Down)
	host.Press(input.Enter)
	if got := awaitValue(t, index.reindexed, "codebase reindex"); got != "/workspace" {
		t.Fatalf("reindexed workspace = %q", got)
	}
	host.Shows(t, "operation op_reindex")
	stop()
}

func TestCodebaseReindexOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := mock.New()
	base := &codebaseServiceStub{reindexed: make(chan string, 1)}
	index := &blockingCodebaseReindexService{
		Service: base, started: make(chan string, 1), release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(index.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", Codebase: index, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/codebase-reindex")
	host.Press(input.Enter)
	host.Shows(t, "Reindex codebase")
	host.Press(input.Down)
	host.Press(input.Enter)
	workspace := awaitValue(t, index.started, "codebase reindex mutation")
	title := "Projection changed during codebase reindex"
	installChangedSessionProjection(t, backend, source, "ses_demo_1", title)
	host.Shows(t, title)
	select {
	case <-index.canceled:
		t.Fatal("session projection replacement canceled the codebase reindex")
	default:
	}
	release()
	if got := awaitValue(t, base.reindexed, "committed codebase reindex"); got != workspace {
		t.Fatalf("reindexed workspace = %q, want %q", got, workspace)
	}
	host.Shows(t, "operation op_reindex")
	stop()
}

func TestCodebaseReindexDoesNotInstallAReaderAfterSessionSwitch(t *testing.T) {
	base := &codebaseServiceStub{reindexed: make(chan string, 1)}
	index := &blockingCodebaseReindexService{
		Service: base, started: make(chan string, 1), release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(index.release) })
	t.Cleanup(release)
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), SessionID: "ses_demo_1", Codebase: index})
	host.Shows(t, "Ask lyra")
	host.Type("/codebase-reindex")
	host.Press(input.Enter)
	host.Shows(t, "Reindex codebase")
	host.Press(input.Down)
	host.Press(input.Enter)
	awaitValue(t, index.started, "codebase reindex mutation")
	host.Hides(t, "Reindex codebase")
	host.Type("/new")
	host.Press(input.Enter)
	host.Shows(t, "session · Untitled session")
	select {
	case <-index.canceled:
		t.Fatal("session switch canceled the admitted codebase reindex")
	default:
	}
	release()
	awaitValue(t, base.reindexed, "committed codebase reindex")
	host.Shows(t, "codebase reindex admitted · op_reindex")
	host.Hides(t, "Codebase index")
	stop()
}

type authoringContextServiceStub struct{}

func (authoringContextServiceStub) Documents(context.Context, string) ([]authoringcontext.Document, error) {
	return []authoringcontext.Document{{Path: "/workspace/AGENTS.md", Title: "Project policy", Scope: authoringcontext.DocumentProjectRoot}}, nil
}

func (authoringContextServiceStub) Recipes(context.Context, string) ([]authoringcontext.Recipe, error) {
	return []authoringcontext.Recipe{{
		Name: "review", Description: "review a target", ArgumentHint: "<target>",
		Body: "Review $1.\nContext: $ARGUMENTS", Scope: authoringcontext.ProjectRecipe, Source: "/workspace/.lyra/recipes/review.md",
	}}, nil
}

func TestAuthoringDocumentsAndRecipeExpansionUseTheUnifiedPromptPath(t *testing.T) {
	runtime := &recordingRuntime{Runtime: mock.New()}
	runtime.Instant = true
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: runtime, Workspace: "/workspace", AuthoringContext: authoringContextServiceStub{},
	})
	host.Shows(t, "Ask lyra")
	host.Type("/agent-docs")
	host.Press(input.Enter)
	host.Shows(t, "/workspace/AGENTS.md")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/recipes")
	host.Press(input.Enter)
	host.Shows(t, "/recipe review <target>")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/recipe rev alpha beta")
	host.Press(input.Enter)
	host.Shows(t, "Recipe · review")
	host.Shows(t, "Review alpha.")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("recipe editor did not survive a minimal viewport")
	}
	host.Send(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	host.Until(t, "the expanded recipe to start a run", func() bool {
		return host.Repaint() && runtime.startCount() > 0
	})
	if got := runtime.startInput().Message.Text; got != "Review alpha.\nContext: alpha beta" {
		t.Fatalf("recipe prompt = %q", got)
	}
	stop()
}

func TestRecipeInvocationPrefersLongestCompleteNameAndSupportsUniquePrefix(t *testing.T) {
	recipes := []authoringcontext.Recipe{{Name: "review"}, {Name: "review code"}, {Name: "summarize"}}
	recipe, arguments, err := resolveRecipeInvocation(recipes, "review code carefully")
	if err != nil || recipe.Name != "review code" || arguments != "carefully" {
		t.Fatalf("multiword invocation = (%q, %q, %v)", recipe.Name, arguments, err)
	}
	recipe, arguments, err = resolveRecipeInvocation(recipes, "sum this file")
	if err != nil || recipe.Name != "summarize" || arguments != "this file" {
		t.Fatalf("prefix invocation = (%q, %q, %v)", recipe.Name, arguments, err)
	}
}

type hookServiceStub struct {
	mu          sync.Mutex
	trusted     bool
	ignoreTrust bool
	changed     chan bool
}

type blockingHookTrustService struct {
	hookpolicy.Service
	started  chan bool
	release  chan struct{}
	canceled chan struct{}
}

func (service *blockingHookTrustService) SetProjectTrust(ctx context.Context, projectRoot string, trusted bool) error {
	service.started <- trusted
	select {
	case <-service.release:
		return service.Service.SetProjectTrust(ctx, projectRoot, trusted)
	case <-ctx.Done():
		close(service.canceled)
		return context.Cause(ctx)
	}
}

func (service *hookServiceStub) Catalog(context.Context, string) (hookpolicy.Catalog, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return hookpolicy.Catalog{ProjectRoot: "/workspace", ProjectTrusted: service.trusted, Hooks: []hookpolicy.Hook{{
		Event: hookpolicy.PreToolUse, Matcher: "shell*", Command: "./check.sh", Scope: hookpolicy.Project,
		Source: "/workspace/.lyra/hooks.json", Active: service.trusted,
	}}}, nil
}

func (service *hookServiceStub) SetProjectTrust(_ context.Context, _ string, trusted bool) error {
	service.mu.Lock()
	if !service.ignoreTrust {
		service.trusted = trusted
	}
	service.mu.Unlock()
	service.changed <- trusted
	return nil
}

func TestHookAuditAndTrustRequireResizeSafeConfirmation(t *testing.T) {
	hooks := &hookServiceStub{changed: make(chan bool, 1)}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Workspace: "/workspace", Hooks: hooks})
	host.Shows(t, "Ask lyra")
	host.Type("/hooks")
	host.Press(input.Enter)
	host.Shows(t, "Shell command")
	host.Shows(t, "./check.sh")
	host.Press(input.Esc)
	host.Shows(t, "Ask lyra")
	host.Type("/hooks-trust")
	host.Press(input.Enter)
	host.Shows(t, "Trust project hooks")
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("hook trust confirmation did not survive a minimal viewport")
	}
	host.Press(input.Down)
	host.Press(input.Enter)
	if trusted := awaitValue(t, hooks.changed, "hook trust change"); !trusted {
		t.Fatal("project trust was not enabled")
	}
	host.Shows(t, "project trust true")
	stop()
}

func TestHookTrustDoesNotReportSuccessWhenAuthoritativeCatalogIsUnchanged(t *testing.T) {
	hooks := &hookServiceStub{ignoreTrust: true, changed: make(chan bool, 1)}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Workspace: "/workspace", Hooks: hooks})
	host.Shows(t, "Ask lyra")
	host.Type("/hooks-trust")
	host.Press(input.Enter)
	host.Shows(t, "Trust project hooks")
	host.Press(input.Down)
	host.Press(input.Enter)
	if trusted := awaitValue(t, hooks.changed, "ignored hook trust change"); !trusted {
		t.Fatal("hook trust request revoked trust")
	}
	host.Shows(t, "update project hook trust failed: verify project hook trust")
	host.Hides(t, "project hook trust updated")
	stop()
}

func TestHookChangeConvergesTheOpenAuditProjection(t *testing.T) {
	hooks := &hookServiceStub{changed: make(chan bool, 1)}
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.HooksChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), Workspace: "/workspace", Hooks: hooks, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "hook change subscription")
	if !slices.Equal(subscription.Topics, []changefeed.Topic{changefeed.HooksChanged}) {
		t.Fatalf("hook subscription = %v", subscription.Topics)
	}
	host.Type("/hooks")
	host.Press(input.Enter)
	host.Shows(t, "project trust false")

	if err := hooks.SetProjectTrust(t.Context(), "/workspace", true); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{Type: changefeed.EventType(changefeed.HooksChanged), Sequence: 1}
	awaitValue(t, source.applied, "hook invalidation")
	host.Shows(t, "project trust true")
	host.Shows(t, "PreToolUse · active")
	stop()
}

func TestHookTrustMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := mock.New()
	base := &hookServiceStub{changed: make(chan bool, 1)}
	hooks := &blockingHookTrustService{
		Service: base, started: make(chan bool, 1), release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(hooks.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", Hooks: hooks, Changes: source,
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/hooks-trust")
	host.Press(input.Enter)
	host.Shows(t, "Trust project hooks")
	host.Press(input.Down)
	host.Press(input.Enter)
	if trusted := awaitValue(t, hooks.started, "hook trust mutation"); !trusted {
		t.Fatal("hook trust mutation revoked trust")
	}
	if _, err := backend.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitValue(t, source.applied, "same-session invalidation")
	select {
	case <-hooks.canceled:
		t.Fatal("session projection replacement canceled the hook trust mutation")
	default:
	}
	release()
	if trusted := awaitValue(t, base.changed, "hook trust change"); !trusted {
		t.Fatal("project trust was not enabled")
	}
	catalog, err := base.Catalog(t.Context(), "/tmp/demo/store")
	if err != nil || !catalog.ProjectTrusted {
		t.Fatalf("hook catalog after trust = (%+v, %v)", catalog, err)
	}
	stop()
}

type feedbackServiceStub struct{ recorded chan feedback.Signal }

type blockingFeedbackService struct {
	feedback.Service
	started  chan feedback.Signal
	release  chan struct{}
	canceled chan struct{}
}

func (service *blockingFeedbackService) Record(ctx context.Context, signal feedback.Signal) error {
	service.started <- signal
	select {
	case <-service.release:
		return service.Service.Record(ctx, signal)
	case <-ctx.Done():
		close(service.canceled)
		return context.Cause(ctx)
	}
}

func (service *feedbackServiceStub) Record(_ context.Context, signal feedback.Signal) error {
	service.recorded <- signal
	return nil
}

func TestFeedbackTargetsLatestDurableAssistantItem(t *testing.T) {
	feedbacks := &feedbackServiceStub{recorded: make(chan feedback.Signal, 1)}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: mock.New(), SessionID: "ses_demo_1", Feedback: feedbacks,
	})
	host.Shows(t, "The fixed sleep races the janitor")
	host.Type("/feedback positive useful explanation")
	host.Press(input.Enter)
	signal := awaitValue(t, feedbacks.recorded, "feedback")
	if signal.SessionID != "ses_demo_1" || signal.RunID == "" || signal.ItemID != "demo_answer" || signal.Rating != feedback.Positive || signal.Text != "useful explanation" {
		t.Fatalf("feedback signal = %+v", signal)
	}
	host.Shows(t, "feedback recorded · positive")
	stop()
}

func TestFeedbackMutationOutlivesSameSessionProjectionReplacement(t *testing.T) {
	backend := mock.New()
	base := &feedbackServiceStub{recorded: make(chan feedback.Signal, 1)}
	feedbacks := &blockingFeedbackService{
		Service: base, started: make(chan feedback.Signal, 1), release: make(chan struct{}), canceled: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(feedbacks.release) })
	t.Cleanup(release)
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, SessionID: "ses_demo_1", Feedback: feedbacks, Changes: source,
	})
	host.Shows(t, "The fixed sleep races the janitor")
	awaitValue(t, source.subscription, "runtime change subscription")
	host.Type("/feedback positive durable signal")
	host.Press(input.Enter)
	signal := awaitValue(t, feedbacks.started, "feedback mutation")
	title := "Projection changed during feedback"
	installChangedSessionProjection(t, backend, source, "ses_demo_1", title)
	host.Shows(t, title)
	select {
	case <-feedbacks.canceled:
		t.Fatal("session projection replacement canceled feedback")
	default:
	}
	release()
	if recorded := awaitValue(t, base.recorded, "committed feedback"); recorded != signal {
		t.Fatalf("recorded feedback = %+v, want %+v", recorded, signal)
	}
	stop()
}
