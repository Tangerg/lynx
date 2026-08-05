package interaction_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func TestStreamingOutputDoesNotDependOnDeltaListeners(t *testing.T) {
	collector := &deltaCollector{}
	engine, err := agent.NewEngine(agent.EngineConfig{
		EventListeners: []agent.EventListener{failingEventListener{}, panickingEventListener{}},
		DeltaListeners: []agent.DeltaListener{collector, failingDeltaListener{}, panickingDeltaListener{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment := newStreamingDeployment(t, responseStream(
		streamTextChunk("hel", ""),
		streamTextChunk("lo", chat.FinishReasonStop),
	))
	input := interactionInput(t, "stream")
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s, termination = %#v", result.Status(), result.Termination())
	}
	erased, _ := result.Output()
	output, err := agent.DecodeOutput[interaction.Output](erased)
	if err != nil {
		t.Fatal(err)
	}
	if output.Response.Text() != "hello" {
		t.Fatalf("final text = %q, want hello", output.Response.Text())
	}
	chunks := collector.Responses()
	if len(chunks) != 2 || chunks[0].Text() != "hel" || chunks[1].Text() != "lo" {
		t.Fatalf("observed chunks = %#v", chunks)
	}
}

func TestStreamingUsesBoundedBestEffortDeltaQueue(t *testing.T) {
	listener := newBlockingDeltaListener()
	t.Cleanup(listener.Release)
	events := &eventRecorder{}
	engine, err := agent.NewEngine(agent.EngineConfig{
		EventListeners:      []agent.EventListener{events},
		DeltaListeners:      []agent.DeltaListener{listener},
		DeltaBufferCapacity: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			if !yield(streamTextChunk("x", ""), nil) {
				return
			}
			<-listener.started
			for range 64 {
				if !yield(streamTextChunk("x", ""), nil) {
					return
				}
			}
			yield(streamTextChunk("x", chat.FinishReasonStop), nil)
		}
	})
	deployment := newStreamingDeployment(t, streamer)
	result, err := engine.Run(context.Background(), deployment, interactionInput(t, "bounded"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s", result.Status())
	}
	if result.Usage().DroppedDeltas == 0 {
		t.Fatal("DroppedDeltas = 0, want an observable bounded-queue drop")
	}
	if !events.Contains("agent.delta.dropped") {
		t.Fatal("missing agent.delta.dropped event")
	}
	listener.Release()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	erased, _ := result.Output()
	output, err := agent.DecodeOutput[interaction.Output](erased)
	if err != nil {
		t.Fatal(err)
	}
	if output.Response.Text() != strings.Repeat("x", 66) {
		t.Fatalf("final response length = %d, want 66", len(output.Response.Text()))
	}
}

func TestRestoringCompletedInteractionDoesNotReplayDeltas(t *testing.T) {
	firstEngine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	deployment := newStreamingDeployment(t, responseStream(streamTextChunk("done", chat.FinishReasonStop)))
	process, err := firstEngine.Start(context.Background(), deployment, interactionInput(t, "restore"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := process.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := firstEngine.Close(); err != nil {
		t.Fatal(err)
	}

	collector := &deltaCollector{}
	restoredEngine, err := agent.NewEngine(agent.EngineConfig{DeltaListeners: []agent.DeltaListener{collector}})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := restored.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if len(collector.Responses()) != 0 {
		t.Fatal("restoration replayed historical model Deltas")
	}
	firstOutput, _ := first.Output()
	secondOutput, _ := second.Output()
	if string(firstOutput.JSON()) != string(secondOutput.JSON()) {
		t.Fatal("restored final Output differs")
	}
}

func newStreamingDeployment(t *testing.T, streamer chat.Streamer) agent.Deployment {
	t.Helper()
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("synchronous model path must not be used")
	})
	client, err := chatclient.New(model, chatclient.Config{Streamer: streamer})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "interaction.stream",
		Description:   "Verify managed streaming Interaction behavior.",
		Version:       "1.0.0",
		MaxModelCalls: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(interaction.DispatcherConfig{
		Client: client, StreamModelResponses: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           definition,
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("interaction-stream-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("interaction-stream-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func interactionInput(t *testing.T, text string) agent.Input {
	t.Helper()
	input, err := agent.EncodeInput(interaction.Input{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart(text))},
	})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func responseStream(chunks ...*chat.Response) chat.Streamer {
	return chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			for _, chunk := range chunks {
				if !yield(chunk, nil) {
					return
				}
			}
		}
	})
}

func streamTextChunk(text string, finish chat.FinishReason) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &message, FinishReason: finish}}}
}

type deltaCollector struct {
	mu        sync.Mutex
	responses []*chat.Response
}

func (collector *deltaCollector) OnDelta(_ context.Context, delta agent.Delta) error {
	decoded, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		return err
	}
	collector.mu.Lock()
	collector.responses = append(collector.responses, decoded.Response())
	collector.mu.Unlock()
	return nil
}

func (collector *deltaCollector) Responses() []*chat.Response {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	responses := make([]*chat.Response, len(collector.responses))
	for index := range collector.responses {
		responses[index] = collector.responses[index].Clone()
	}
	return responses
}

type failingDeltaListener struct{}

func (failingDeltaListener) OnDelta(context.Context, agent.Delta) error {
	return errors.New("delta listener failed")
}

type panickingDeltaListener struct{}

func (panickingDeltaListener) OnDelta(context.Context, agent.Delta) error {
	panic("delta listener panicked")
}

type failingEventListener struct{}

func (failingEventListener) OnEvent(context.Context, agent.Event) error {
	return errors.New("event listener failed")
}

type panickingEventListener struct{}

func (panickingEventListener) OnEvent(context.Context, agent.Event) error {
	panic("event listener panicked")
}

type blockingDeltaListener struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingDeltaListener() *blockingDeltaListener {
	return &blockingDeltaListener{started: make(chan struct{}), release: make(chan struct{})}
}

func (listener *blockingDeltaListener) OnDelta(context.Context, agent.Delta) error {
	listener.startedOnce.Do(func() { close(listener.started) })
	<-listener.release
	return nil
}

func (listener *blockingDeltaListener) Release() {
	listener.releaseOnce.Do(func() { close(listener.release) })
}

type eventRecorder struct {
	mu    sync.Mutex
	names []string
}

func (recorder *eventRecorder) OnEvent(_ context.Context, event agent.Event) error {
	recorder.mu.Lock()
	recorder.names = append(recorder.names, event.Name())
	recorder.mu.Unlock()
	return nil
}

func (recorder *eventRecorder) Contains(name string) bool {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, candidate := range recorder.names {
		if candidate == name {
			return true
		}
	}
	return false
}
