package interaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func TestConcurrentToolsRespectLimitAndCommitInModelOrder(t *testing.T) {
	started := make(chan string, 3)
	var active atomic.Int32
	var maximum atomic.Int32
	tools := make([]tool.Tool, 0, 3)
	releases := make(map[string]*toolRelease)
	for _, name := range []string{"first", "second", "third"} {
		release := newToolRelease()
		releases[name] = release
		t.Cleanup(release.Release)
		tools = append(tools, &scheduledTool{
			name: name,
			call: func(context.Context, string) (string, error) {
				current := active.Add(1)
				defer active.Add(-1)
				for previous := maximum.Load(); current > previous && !maximum.CompareAndSwap(previous, current); previous = maximum.Load() {
				}
				started <- name
				<-release.done
				return name + " result", nil
			},
		})
	}
	model := &orderedBatchModel{names: []string{"first", "second", "third"}}
	process, engine := startConcurrentInteraction(t, model, tools, 2)

	firstStarted := <-started
	secondStarted := <-started
	if firstStarted == secondStarted {
		t.Fatalf("started the same Tool twice: %q", firstStarted)
	}
	select {
	case thirdStarted := <-started:
		t.Fatalf("third Tool %q exceeded the concurrency limit", thirdStarted)
	default:
	}

	releases[firstStarted].Release()
	thirdStarted := <-started
	releases[secondStarted].Release()
	releases[thirdStarted].Release()
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s", result.Status())
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent calls = %d, want 2", maximum.Load())
	}
}

func TestConcurrentToolsWithSameKeyDoNotOverlap(t *testing.T) {
	started := make(chan string, 2)
	firstRelease := newToolRelease()
	t.Cleanup(firstRelease.Release)
	first := &scheduledTool{
		name: "first", key: "resource:shared",
		call: func(context.Context, string) (string, error) {
			started <- "first"
			<-firstRelease.done
			return "first result", nil
		},
	}
	second := &scheduledTool{
		name: "second", key: "resource:shared",
		call: func(context.Context, string) (string, error) {
			started <- "second"
			return "second result", nil
		},
	}
	process, engine := startConcurrentInteraction(
		t,
		&orderedBatchModel{names: []string{"first", "second"}},
		[]tool.Tool{first, second},
		2,
	)
	if got := <-started; got != "first" {
		t.Fatalf("first started Tool = %q", got)
	}
	select {
	case got := <-started:
		t.Fatalf("same-key Tool %q overlapped", got)
	default:
	}
	firstRelease.Release()
	if got := <-started; got != "second" {
		t.Fatalf("second started Tool = %q", got)
	}
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s", result.Status())
	}
}

func TestZeroToolConcurrencyLimitKeepsDeclaredToolsSerial(t *testing.T) {
	started := make(chan string, 2)
	firstRelease := newToolRelease()
	t.Cleanup(firstRelease.Release)
	first := &scheduledTool{
		name: "first",
		call: func(context.Context, string) (string, error) {
			started <- "first"
			<-firstRelease.done
			return "first result", nil
		},
	}
	second := &scheduledTool{
		name: "second",
		call: func(context.Context, string) (string, error) {
			started <- "second"
			return "second result", nil
		},
	}
	process, engine := startConcurrentInteraction(
		t,
		&orderedBatchModel{names: []string{"first", "second"}},
		[]tool.Tool{first, second},
		0,
	)
	if got := <-started; got != "first" {
		t.Fatalf("first started Tool = %q", got)
	}
	select {
	case got := <-started:
		t.Fatalf("zero-limit Tool %q overlapped", got)
	default:
	}
	firstRelease.Release()
	if got := <-started; got != "second" {
		t.Fatalf("second started Tool = %q", got)
	}
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s", result.Status())
	}
}

func TestUndeclaredToolIsAnExclusiveBatchBarrier(t *testing.T) {
	started := make(chan string, 3)
	firstRelease := newToolRelease()
	secondRelease := newToolRelease()
	t.Cleanup(firstRelease.Release)
	t.Cleanup(secondRelease.Release)
	first := &scheduledTool{
		name: "first",
		call: func(context.Context, string) (string, error) {
			started <- "first"
			<-firstRelease.done
			return "first result", nil
		},
	}
	second := &exclusiveTool{
		name: "exclusive",
		call: func(context.Context, string) (string, error) {
			started <- "exclusive"
			<-secondRelease.done
			return "exclusive result", nil
		},
	}
	third := &scheduledTool{
		name: "third",
		call: func(context.Context, string) (string, error) {
			started <- "third"
			return "third result", nil
		},
	}
	process, engine := startConcurrentInteraction(
		t,
		&orderedBatchModel{names: []string{"first", "exclusive", "third"}},
		[]tool.Tool{first, second, third},
		3,
	)
	if got := <-started; got != "first" {
		t.Fatalf("first started Tool = %q", got)
	}
	select {
	case got := <-started:
		t.Fatalf("Tool %q crossed the exclusive barrier", got)
	default:
	}
	firstRelease.Release()
	if got := <-started; got != "exclusive" {
		t.Fatalf("second started Tool = %q", got)
	}
	select {
	case got := <-started:
		t.Fatalf("Tool %q overlapped the exclusive Tool", got)
	default:
	}
	secondRelease.Release()
	if got := <-started; got != "third" {
		t.Fatalf("third started Tool = %q", got)
	}
	result, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s", result.Status())
	}
}

func TestDispatcherRejectsNegativeToolConcurrencyLimit(t *testing.T) {
	client, err := chatclient.New(&orderedBatchModel{names: []string{"unused"}}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "interaction.invalid_concurrency", Description: "Reject an invalid Tool concurrency limit.",
		Version: "1.0.0", MaxModelCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: client, MaxConcurrentToolCalls: -1,
	})
	if !errors.Is(err, interaction.ErrInvalidDispatcherConfig) {
		t.Fatalf("error = %v, want ErrInvalidDispatcherConfig", err)
	}
}

func TestParallelToolInputRequestMakesBatchOutcomeUnknown(t *testing.T) {
	var siblingCalls atomic.Int32
	requesting := &scheduledTool{
		name: "requesting",
		call: func(context.Context, string) (string, error) {
			return "", interaction.RequireToolInput(
				json.RawMessage(`"continue?"`),
				json.RawMessage(`{"type":"boolean"}`),
				json.RawMessage(`{"stage":1}`),
			)
		},
	}
	sibling := &scheduledTool{
		name: "sibling",
		call: func(context.Context, string) (string, error) {
			siblingCalls.Add(1)
			return "sibling result", nil
		},
	}
	process, engine := startConcurrentInteraction(
		t,
		&orderedBatchModel{names: []string{"requesting", "sibling"}},
		[]tool.Tool{requesting, sibling},
		2,
	)
	unknown := waitForUnknownEffects(t, process)
	if len(unknown) != 1 {
		t.Fatalf("unknown EffectIDs = %v, want one", unknown)
	}
	if siblingCalls.Load() != 1 {
		t.Fatalf("sibling calls = %d, want 1", siblingCalls.Load())
	}
	if process.Status() == agent.StatusWaiting {
		t.Fatal("unsafe parallel input request was exposed as a resumable checkpoint")
	}
	if err := process.Kill(context.Background(), "parallel Tool violated its declaration"); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
}

type scheduledTool struct {
	name string
	key  string
	call func(context.Context, string) (string, error)
}

func (value *scheduledTool) Definition() chat.ToolDefinition {
	return concurrencyToolDefinition(value.name)
}

func (value *scheduledTool) Call(ctx context.Context, arguments string) (string, error) {
	return value.call(ctx, arguments)
}

func (value *scheduledTool) ConcurrencyKey(string) (string, bool) { return value.key, true }

type exclusiveTool struct {
	name string
	call func(context.Context, string) (string, error)
}

func (value *exclusiveTool) Definition() chat.ToolDefinition {
	return concurrencyToolDefinition(value.name)
}

func (value *exclusiveTool) Call(ctx context.Context, arguments string) (string, error) {
	return value.call(ctx, arguments)
}

type toolRelease struct {
	done chan struct{}
	once sync.Once
}

func newToolRelease() *toolRelease { return &toolRelease{done: make(chan struct{})} }

func (release *toolRelease) Release() { release.once.Do(func() { close(release.done) }) }

type orderedBatchModel struct {
	mu    sync.Mutex
	calls int
	names []string
}

func (model *orderedBatchModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	if model.calls == 1 {
		calls := make([]chat.ToolCall, len(model.names))
		for index, name := range model.names {
			calls[index] = chat.ToolCall{ID: "call_" + name, Name: name, Arguments: `{}`}
		}
		return toolBatchResponse(calls...), nil
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != chat.RoleTool || len(last.Parts) != len(model.names) {
		return nil, errors.New("model did not receive the complete ordered Tool batch")
	}
	for index, name := range model.names {
		result := last.Parts[index].ToolResult
		if result == nil || result.ID != "call_"+name || result.Name != name || result.Result != name+" result" {
			return nil, errors.New("Tool results did not preserve model call order")
		}
	}
	return textResponse("done"), nil
}

func startConcurrentInteraction(
	t *testing.T,
	model chat.Model,
	tools []tool.Tool,
	maxConcurrent int,
) (*agent.Process, *agent.Engine) {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "interaction.concurrent", Description: "Verify bounded Tool concurrency.",
		Version: "1.0.0", MaxModelCalls: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: client, Tools: tools, MaxConcurrentToolCalls: maxConcurrent,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("interaction-concurrency-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("interaction-concurrency-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Start(context.Background(), deployment, interactionInput(t, "run tools"))
	if err != nil {
		_ = engine.Close()
		t.Fatal(err)
	}
	return process, engine
}

func concurrencyToolDefinition(name string) chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: name, Description: "A deterministic concurrency contract test Tool.",
		InputSchema: []byte(`{"type":"object","additionalProperties":false}`),
	}
}

func toolBatchResponse(calls ...chat.ToolCall) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for index := range calls {
		call := calls[index]
		parts[index] = chat.NewToolCallPart(call)
	}
	return &chat.Response{Choices: []chat.Choice{{
		Index: 0, Message: pointerMessage(chat.NewAssistantMessage(parts...)), FinishReason: "tool_calls",
	}}}
}

func pointerMessage(message chat.Message) *chat.Message { return &message }

func waitForUnknownEffects(t *testing.T, process *agent.Process) []agent.EffectID {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ids, err := process.UnknownEffectIDs(context.Background())
		if err == nil && len(ids) > 0 {
			return ids
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Process never exposed an unknown Tool Effect")
	return nil
}

var _ interaction.ConcurrentTool = (*scheduledTool)(nil)
