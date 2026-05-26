package ipc_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"

	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/transport/ipc"
)

// TestHealthz_RoundTrip is the smoke test — a healthz request
// gets a result frame in response.
func TestHealthz_RoundTrip(t *testing.T) {
	srv, stdin, frames, close := newPipedServer(t, approval.ModeYolo)
	defer close()

	writeRequest(t, stdin, `{"id":"h1","method":"healthz"}`)
	got := readUntil(t, frames, "h1", isResultOrError, 2*time.Second)
	if got.Error != nil {
		t.Fatalf("healthz returned error: %+v", got.Error)
	}
	_ = srv // keep alive
}

// TestSessionsCreateThenGet uses the IPC surface to round-trip a
// session. Verifies that sessions.* methods stay coherent across
// requests through the same runtime.
func TestSessionsCreateThenGet(t *testing.T) {
	_, stdin, frames, close := newPipedServer(t, approval.ModeYolo)
	defer close()

	writeRequest(t, stdin, `{"id":"c1","method":"sessions.create","params":{"title":"ipc test"}}`)
	created := readUntil(t, frames, "c1", isResultOrError, 2*time.Second)
	if created.Error != nil {
		t.Fatalf("create error: %+v", created.Error)
	}
	id, _ := created.Result.(map[string]any)["id"].(string)
	if id == "" {
		t.Fatalf("create result missing id: %+v", created.Result)
	}

	writeRequest(t, stdin, `{"id":"g1","method":"sessions.get","params":{"id":"`+id+`"}}`)
	got := readUntil(t, frames, "g1", isResultOrError, 2*time.Second)
	if got.Error != nil {
		t.Fatalf("get error: %+v", got.Error)
	}
	if got.Result.(map[string]any)["id"] != id {
		t.Errorf("get returned wrong id: %+v", got.Result)
	}
}

// TestAgentRunStreams drives one full turn through the IPC
// transport — multiple event frames followed by done.
func TestAgentRunStreams(t *testing.T) {
	_, stdin, frames, close := newPipedServer(t, approval.ModeYolo)
	defer close()

	writeRequest(t, stdin, `{"id":"r1","method":"agent.run","params":{"message":"say lyra via bash"}}`)

	var eventTypes []string
	deadline := time.After(5 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.ID != "r1" {
				continue
			}
			if f.Done {
				goto check
			}
			if f.Error != nil {
				t.Fatalf("run error: %+v", f.Error)
			}
			if len(f.Event) > 0 {
				var payload struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(f.Event, &payload)
				if payload.Type != "" {
					eventTypes = append(eventTypes, payload.Type)
				}
			}
		case <-deadline:
			t.Fatalf("timeout waiting for done; got events: %v", eventTypes)
		}
	}
check:
	if len(eventTypes) == 0 {
		t.Fatal("no events seen")
	}
	if eventTypes[0] != "RUN_STARTED" {
		t.Errorf("first event = %q, want RUN_STARTED (all: %v)", eventTypes[0], eventTypes)
	}
	last := eventTypes[len(eventTypes)-1]
	if last != "RUN_FINISHED" && last != "RUN_ERROR" {
		t.Errorf("last event = %q, want RUN_FINISHED|RUN_ERROR (all: %v)", last, eventTypes)
	}
}

// TestMethodNotFound returns METHOD_NOT_FOUND on unknown method.
func TestMethodNotFound(t *testing.T) {
	_, stdin, frames, close := newPipedServer(t, approval.ModeYolo)
	defer close()

	writeRequest(t, stdin, `{"id":"x1","method":"not.a.method"}`)
	got := readUntil(t, frames, "x1", isResultOrError, time.Second)
	if got.Error == nil || got.Error.Code != "METHOD_NOT_FOUND" {
		t.Fatalf("error = %+v, want METHOD_NOT_FOUND", got.Error)
	}
}

// ------------------------------------------------------------------
// Test harness
// ------------------------------------------------------------------

type frame struct {
	ID     string          `json:"id"`
	Result any             `json:"result,omitempty"`
	Event  json.RawMessage `json:"event,omitempty"`
	Done   bool            `json:"done,omitempty"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// newPipedServer boots an IPC server with io.Pipe stdin/stdout —
// the test pushes JSON-RPC lines into stdin and reads parsed
// frames off the returned channel.
func newPipedServer(t *testing.T, mode approval.Mode) (
	*ipc.Server,
	io.Writer,
	<-chan frame,
	func(),
) {
	t.Helper()

	client, _ := chatmodel.NewClient(newStubChatModel())
	rt, err := lyraruntime.New(lyraruntime.Config{
		ChatClient:   client,
		ApprovalMode: mode,
	})
	if err != nil {
		t.Fatalf("runtime.New: %v", err)
	}

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	srv, err := ipc.NewServer(ipc.Config{
		Runtime: rt,
		In:      stdinR,
		Out:     stdoutW,
	})
	if err != nil {
		t.Fatalf("ipc.NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx)
		close(serveDone)
	}()

	frames := make(chan frame, 64)
	var readDone sync.WaitGroup
	readDone.Add(1)
	go func() {
		defer readDone.Done()
		defer close(frames)
		buf := bufio.NewScanner(stdoutR)
		buf.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for buf.Scan() {
			var f frame
			if err := json.Unmarshal(buf.Bytes(), &f); err != nil {
				continue
			}
			frames <- f
		}
		// Scanner.Err is intentionally discarded — pipe closed
		// by cleanup() during test teardown produces an
		// io.ErrClosedPipe that the test doesn't care about.
		_ = buf.Err()
	}()

	cleanup := func() {
		cancel()
		_ = stdinW.Close()
		_ = stdoutW.Close()
		<-serveDone
		_ = stdoutR.Close()
		_ = rt.Close()
	}
	return srv, stdinW, frames, cleanup
}

func writeRequest(t *testing.T, w io.Writer, line string) {
	t.Helper()
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// readUntil drains frames waiting for a non-event frame matching
// id; events are skipped. Fails the test on timeout.
func readUntil(t *testing.T, frames <-chan frame, id string, pred func(frame) bool, timeout time.Duration) frame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("frames channel closed waiting for %s", id)
			}
			if f.ID == id && pred(f) {
				return f
			}
		case <-deadline:
			t.Fatalf("timeout waiting for id %s", id)
		}
	}
}

func isResultOrError(f frame) bool { return f.Result != nil || f.Error != nil }

// ------------------------------------------------------------------
// Stub model — duplicated minimally from chat/impl_test.go
// ------------------------------------------------------------------

type stubChatModel struct{ defaults *chatmodel.Options }

func newStubChatModel() *stubChatModel {
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *stubChatModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *stubChatModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	if hasToolMsg(req.Messages) {
		return makeText("ok")
	}
	return makeToolCall("bash", `{"command":"echo lyra"}`)
}

func (m *stubChatModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func hasToolMsg(messages []chatmodel.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chatmodel.MessageTypeTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(text),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	calls := []*chatmodel.ToolCallPart{{ID: "c1", Name: name, Arguments: args}}
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(calls),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonToolCalls},
		},
		&chatmodel.ResponseMetadata{},
	)
}

// strings import retained for potential future use; harmless.
var _ = strings.TrimSpace
var _ = bytes.NewBufferString
