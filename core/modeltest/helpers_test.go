package modeltest_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/modeltest"
	"github.com/Tangerg/scope/core/rerank"
)

func helloRequest(t *testing.T) *chat.Request {
	t.Helper()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func chunkResponse(id string) *chat.Response {
	return &chat.Response{Metadata: &chat.ResponseMetadata{ID: id}}
}

// blockingChat is the minimal provider shape the behavior suite is written
// against: it holds an HTTP request open until the caller's context is
// released, which is exactly how a real SDK surfaces cancellation.
type blockingChat struct {
	url string
}

func (b blockingChat) Call(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	return chunkResponse("call"), nil
}

func (b blockingChat) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if err := b.wait(ctx); err != nil {
			yield(nil, err)
			return
		}
		yield(chunkResponse("stream"), nil)
	}
}

func (b blockingChat) wait(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// streamingChat yields one response, then blocks until the caller stops
// iterating or the context is released.
type streamingChat struct {
	url string
}

func (s streamingChat) Stream(ctx context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
		if err != nil {
			yield(nil, err)
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			yield(nil, err)
			return
		}
		defer response.Body.Close()
		if _, err := bufio.NewReader(response.Body).ReadString('\n'); err != nil {
			yield(nil, err)
			return
		}
		if !yield(chunkResponse("stream"), nil) {
			return
		}
		<-ctx.Done()
		yield(nil, ctx.Err())
	}
}

// failingChat yields one good response and then a terminal error, which is the
// shape "first error terminates" is written against.
type failingChat struct{}

func (failingChat) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if !yield(chunkResponse("stream"), nil) {
			return
		}
		yield(nil, errors.New("provider failed"))
	}
}

func writeInitialLine(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ready\n"))
	writer.(http.Flusher).Flush()
}

// TestChatBehaviorSuite runs the shared behavior contract against fakes that
// honor it. The suite is the harness every provider module is held to, so a
// regression in the harness itself has to fail here rather than silently stop
// checking providers.
func TestChatBehaviorSuite(t *testing.T) {
	modeltest.ChatBehaviorSuite{
		Request: helloRequest,
		CallCancellation: func(t *testing.T) modeltest.CallBehaviorCase {
			server, lifecycle := modeltest.NewBlockingServer(t, writeInitialLine)
			return modeltest.CallBehaviorCase{
				Model:     blockingChat{url: server.URL},
				Lifecycle: lifecycle,
			}
		},
		StreamCancellation: func(t *testing.T) modeltest.StreamBehaviorCase {
			server, lifecycle := modeltest.NewBlockingServer(t, writeInitialLine)
			return modeltest.StreamBehaviorCase{
				Streamer:  blockingChat{url: server.URL},
				Lifecycle: lifecycle,
			}
		},
		EarlyStop: func(t *testing.T) modeltest.StreamBehaviorCase {
			server, lifecycle := modeltest.NewBlockingServer(t, writeInitialLine)
			return modeltest.StreamBehaviorCase{
				Streamer:  streamingChat{url: server.URL},
				Lifecycle: lifecycle,
			}
		},
		FirstError: func(*testing.T) chat.Streamer { return failingChat{} },
	}.Run(t)
}

// TestNewBlockingServerUsesADefaultInitialWrite covers the branch where a
// caller does not supply an initial write, which providers that stream nothing
// before their first chunk rely on.
func TestNewBlockingServerUsesADefaultInitialWrite(t *testing.T) {
	server, lifecycle := modeltest.NewBlockingServer(t, nil)
	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			done <- err
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- err
			return
		}
		defer response.Body.Close()
		_, err = response.Body.Read(make([]byte, 1))
		done <- err
	}()

	select {
	case <-lifecycle.Started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("blocking server never signaled start")
	}
	cancel()
	<-done
	select {
	case <-lifecycle.Stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking server never signaled stop")
	}
}

type fakeEmbedding struct{}

func (fakeEmbedding) Call(_ context.Context, request *embedding.Request) (*embedding.Response, error) {
	outputs := make([]*embedding.Output, 0, len(request.Texts))
	for range request.Texts {
		output, err := embedding.NewOutput([]float64{0.1, 0.2}, nil)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return embedding.NewResponse(outputs, nil)
}

// TestRunEmbeddingContract exercises the embedding harness the same way a
// provider module does, including the URL-path assertion.
func TestRunEmbeddingContract(t *testing.T) {
	built := false
	modeltest.RunEmbeddingContract(t, modeltest.EmbeddingContract{
		ModelID:      "embedding-model",
		Response:     `{"outputs":[]}`,
		ExpectedPath: "/embeddings",
		Build: func(t *testing.T, baseURL string) embedding.Model {
			built = true
			if baseURL == "" {
				t.Fatal("contract passed an empty base URL")
			}
			return probeEmbedding{baseURL: baseURL}
		},
	})
	if !built {
		t.Fatal("contract never built the model")
	}
}

func TestRunEmbeddingContractAllowsUnspecifiedPath(t *testing.T) {
	modeltest.RunEmbeddingContract(t, modeltest.EmbeddingContract{
		ModelID:  "embedding-model",
		Response: `{"outputs":[]}`,
		Build: func(*testing.T, string) embedding.Model {
			return fakeEmbedding{}
		},
	})
}

type probeRerank struct{ baseURL string }

func (p probeRerank) Call(ctx context.Context, request *rerank.Request) (*rerank.Response, error) {
	wireRequest := struct {
		Model     string   `json:"model"`
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
		TopN      int      `json:"top_n"`
	}{
		Model: "rerank-model", Query: request.Query, Documents: request.Documents, TopN: request.Options.TopK,
	}
	payload, err := json.Marshal(wireRequest)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	return rerank.NewResponse([]*rerank.Result{{Index: 0, Score: 0.9}, {Index: 1, Score: 0.8}}, nil)
}

func TestRunRerankContract(t *testing.T) {
	built := false
	modeltest.RunRerankContract(t, modeltest.RerankContract{
		ModelID:      "rerank-model",
		Response:     `{"results":[]}`,
		ExpectedPath: "/rerank",
		Build: func(t *testing.T, baseURL string) rerank.Model {
			built = true
			if baseURL == "" {
				t.Fatal("contract passed an empty base URL")
			}
			return probeRerank{baseURL: baseURL}
		},
	})
	if !built {
		t.Fatal("contract never built the model")
	}
}

// probeEmbedding hits the contract's mock server so the path assertion has
// something to observe, then answers from the request itself.
type probeEmbedding struct{ baseURL string }

func (p probeEmbedding) Call(ctx context.Context, request *embedding.Request) (*embedding.Response, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return fakeEmbedding{}.Call(ctx, request)
}

func TestMuxServerRoutesByMethodAndPath(t *testing.T) {
	var counter modeltest.PollCounter
	server := modeltest.MuxServer(
		modeltest.Route{Method: http.MethodPost, Contains: "/transcript", Handle: func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("created"))
		}},
		modeltest.Route{Method: http.MethodGet, Contains: "/transcript", Handle: func(writer http.ResponseWriter, _ *http.Request) {
			if counter.Inc() < 2 {
				_, _ = writer.Write([]byte("processing"))
				return
			}
			_, _ = writer.Write([]byte("completed"))
		}},
		modeltest.Route{Handle: func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte("fallback"))
		}},
	)
	t.Cleanup(server.Close)

	if body := post(t, server.URL+"/v2/transcript"); body != "created" {
		t.Fatalf("POST body = %q", body)
	}
	if body := get(t, server.URL+"/v2/transcript/job-1"); body != "processing" {
		t.Fatalf("first poll body = %q", body)
	}
	if body := get(t, server.URL+"/v2/transcript/job-1"); body != "completed" {
		t.Fatalf("second poll body = %q", body)
	}
	if counter.N() != 2 {
		t.Fatalf("PollCounter.N = %d, want 2", counter.N())
	}
	if body := get(t, server.URL+"/unmatched"); body != "fallback" {
		t.Fatalf("fallback body = %q", body)
	}
}

// TestMuxServerReportsAnUnroutedRequest keeps a missing route loud instead of
// letting a provider test read an empty 200 as a valid answer.
func TestMuxServerReportsAnUnroutedRequest(t *testing.T) {
	server := modeltest.MuxServer(modeltest.Route{Method: http.MethodPost, Contains: "/only-post"})
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/other")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
}

func TestOpenAISSEServerFramesChunksAndTerminates(t *testing.T) {
	server := modeltest.OpenAISSEServer([]string{`{"id":"1"}`, `{"id":"2"}`})
	t.Cleanup(server.Close)

	body := get(t, server.URL)
	want := "data: {\"id\":\"1\"}\n\ndata: {\"id\":\"2\"}\n\ndata: [DONE]\n\n"
	if body != want {
		t.Fatalf("SSE body = %q, want %q", body, want)
	}
}

func TestAnthropicSSEServerNamesEachEvent(t *testing.T) {
	server := modeltest.AnthropicSSEServer([]modeltest.AnthropicEvent{
		{Event: "message_start", Data: `{"type":"message_start"}`},
		{Event: "message_stop", Data: `{"type":"message_stop"}`},
	})
	t.Cleanup(server.Close)

	body := get(t, server.URL)
	if !strings.Contains(body, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n") {
		t.Fatalf("SSE body = %q", body)
	}
	if !strings.HasSuffix(body, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n") {
		t.Fatalf("SSE body did not end with the stop event: %q", body)
	}
}

func TestJSONServerReportsStatusBodyAndRequest(t *testing.T) {
	var seenPath, seenMethod string
	server := modeltest.JSONServer(http.StatusTeapot, `{"error":"nope"}`, func(request *http.Request) {
		seenPath = request.URL.Path
		seenMethod = request.Method
	})
	t.Cleanup(server.Close)

	response, err := http.Post(server.URL+"/v1/chat", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if seenPath != "/v1/chat" || seenMethod != http.MethodPost {
		t.Fatalf("inspection saw %s %s", seenMethod, seenPath)
	}
}

func TestBinaryServerReportsContentTypeAndBytes(t *testing.T) {
	inspected := false
	server := modeltest.BinaryServer(http.StatusOK, "audio/mpeg", []byte{0xFF, 0xFB}, func(*http.Request) {
		inspected = true
	})
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "audio/mpeg" {
		t.Fatalf("content type = %q", got)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload[0] != 0xFF || payload[1] != 0xFB {
		t.Fatalf("payload = %v", payload)
	}
	if !inspected {
		t.Fatal("inspection was not called")
	}
}

func TestCollectStopsAtTheFirstError(t *testing.T) {
	boom := errors.New("boom")
	sequence := func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		if !yield(0, boom) {
			return
		}
		yield(2, nil)
	}
	values, err := modeltest.Collect(iter.Seq2[int, error](sequence))
	if !errors.Is(err, boom) {
		t.Fatalf("Collect error = %v, want %v", err, boom)
	}
	if len(values) != 1 || values[0] != 1 {
		t.Fatalf("Collect values = %v, want the values before the error", values)
	}
}

func TestCollectDrainsACompleteSequence(t *testing.T) {
	sequence := func(yield func(int, error) bool) {
		for value := range 3 {
			if !yield(value, nil) {
				return
			}
		}
	}
	values, err := modeltest.Collect(iter.Seq2[int, error](sequence))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("Collect values = %v", values)
	}
}

func TestCollectNStopsEarlyAndPropagatesErrors(t *testing.T) {
	delivered := 0
	sequence := func(yield func(int, error) bool) {
		for value := range 10 {
			delivered++
			if !yield(value, nil) {
				return
			}
		}
	}
	values, err := modeltest.CollectN(iter.Seq2[int, error](sequence), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("CollectN values = %v, want 2", values)
	}
	if delivered != 2 {
		t.Fatalf("CollectN pulled %d items, want 2", delivered)
	}

	boom := errors.New("boom")
	failing := func(yield func(int, error) bool) { yield(0, boom) }
	if _, err = modeltest.CollectN(iter.Seq2[int, error](failing), 5); !errors.Is(err, boom) {
		t.Fatalf("CollectN error = %v, want %v", err, boom)
	}

	short := func(yield func(int, error) bool) { yield(1, nil) }
	values, err = modeltest.CollectN(iter.Seq2[int, error](short), 5)
	if err != nil || len(values) != 1 {
		t.Fatalf("CollectN over a short sequence = %v, %v", values, err)
	}
}

func TestEnvironmentHelpersReadPresentValues(t *testing.T) {
	t.Setenv("SCOPE_TEST_PROBE_KEY", "secret")
	if got := modeltest.RequireKey(t, "probe"); got != "secret" {
		t.Fatalf("RequireKey = %q", got)
	}

	t.Setenv("SCOPE_TEST_PROBE_REGION", "eu")
	if got := modeltest.RequireEnv(t, "SCOPE_TEST_PROBE_REGION"); got != "eu" {
		t.Fatalf("RequireEnv = %q", got)
	}

	value, found := modeltest.LookupEnv("SCOPE_TEST_PROBE_REGION")
	if !found || value != "eu" {
		t.Fatalf("LookupEnv = %q, %t", value, found)
	}
	if _, found := modeltest.LookupEnv("SCOPE_TEST_PROBE_ABSENT"); found {
		t.Fatal("LookupEnv reported an unset variable as present")
	}
	t.Setenv("SCOPE_TEST_PROBE_EMPTY", "")
	if _, found := modeltest.LookupEnv("SCOPE_TEST_PROBE_EMPTY"); found {
		t.Fatal("LookupEnv reported an empty variable as present")
	}
}

// TestRunIntegrationEmbeddingSkipsWithoutAKey documents the guard every
// integration probe relies on: no key means skip, never fail.
func TestRunIntegrationEmbeddingSkipsWithoutAKey(t *testing.T) {
	t.Setenv("SCOPE_TEST_ABSENTPROVIDER_KEY", "")
	built := false
	skipped := t.Run("probe", func(t *testing.T) {
		modeltest.RunIntegrationEmbedding(t, modeltest.IntegrationEmbeddingProbe{
			Provider: "absentprovider",
			Build: func(*testing.T, string) embedding.Model {
				built = true
				return fakeEmbedding{}
			},
		})
	})
	if !skipped {
		t.Fatal("the probe failed instead of skipping")
	}
	if built {
		t.Fatal("the probe built a model without a key")
	}
}

func TestRunIntegrationRerankSkipsWithoutAKey(t *testing.T) {
	t.Setenv("SCOPE_TEST_ABSENTRERANK_KEY", "")
	built := false
	skipped := t.Run("probe", func(t *testing.T) {
		modeltest.RunIntegrationRerank(t, modeltest.IntegrationRerankProbe{
			Provider: "absentrerank",
			Build: func(*testing.T, string) rerank.Model {
				built = true
				return probeRerank{}
			},
		})
	})
	if !skipped {
		t.Fatal("the probe failed instead of skipping")
	}
	if built {
		t.Fatal("the probe built a model without a key")
	}
}

func TestRunIntegrationRerankCallsTheConfiguredModel(t *testing.T) {
	t.Setenv("SCOPE_TEST_RERANKPROBE_KEY", "secret")
	built := false
	called := false
	modeltest.RunIntegrationRerank(t, modeltest.IntegrationRerankProbe{
		Provider: "rerankprobe",
		Build: func(_ *testing.T, key string) rerank.Model {
			built = key == "secret"
			return rerank.ModelFunc(func(_ context.Context, request *rerank.Request) (*rerank.Response, error) {
				called = true
				results := make([]*rerank.Result, len(request.Documents))
				for index := range request.Documents {
					results[index] = &rerank.Result{Index: index, Score: rerank.Score(1 - float64(index)/2)}
				}
				return rerank.NewResponse(results, nil)
			})
		},
	})
	if !built || !called {
		t.Fatalf("integration probe = built:%t called:%t", built, called)
	}
}

func TestWithTimeoutDerivesFromTheTestContext(t *testing.T) {
	ctx, cancel := modeltest.WithTimeout(t, time.Minute)
	defer cancel()
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		t.Fatal("WithTimeout returned a context without a deadline")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v", ctx.Err())
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func post(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
