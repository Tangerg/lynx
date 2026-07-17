package http

import (
	"context"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestObservabilityRecordsContainedPanicResponse(t *testing.T) {
	exporter := captureHTTPSpans(t)

	cause := errors.New("handler sentinel")
	handler := (&Server{}).observability(nethttp.HandlerFunc(func(nethttp.ResponseWriter, *nethttp.Request) {
		panic(cause)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(nethttp.MethodGet, "http://example.test/panic", nil))

	if response.Code != nethttp.StatusInternalServerError {
		t.Fatalf("response status = %d, want %d", response.Code, nethttp.StatusInternalServerError)
	}
	if !strings.Contains(response.Body.String(), `"error":"internal error"`) {
		t.Fatalf("response body = %q, want flat internal error", response.Body.String())
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want Error", span.Status.Code)
	}
	var status, bodySize int64
	for _, attr := range span.Attributes {
		switch string(attr.Key) {
		case "http.response.status_code":
			status = attr.Value.AsInt64()
		case "http.response.body.size":
			bodySize = attr.Value.AsInt64()
		}
	}
	if status != nethttp.StatusInternalServerError {
		t.Fatalf("span response status = %d, want %d", status, nethttp.StatusInternalServerError)
	}
	if bodySize != int64(response.Body.Len()) {
		t.Fatalf("span body size = %d, want %d", bodySize, response.Body.Len())
	}

	foundPanic := false
	for _, event := range span.Events {
		if event.Name != "exception" {
			continue
		}
		for _, attr := range event.Attributes {
			if string(attr.Key) == "exception.message" && strings.Contains(attr.Value.AsString(), "http handler panicked: handler sentinel") {
				foundPanic = true
			}
		}
	}
	if !foundPanic {
		t.Fatal("span has no panic exception event")
	}
	if err := handlerPanicError(cause); !errors.Is(err, cause) {
		t.Fatalf("handlerPanicError() = %v, want wrapped cause", err)
	}
}

func TestObservabilityDoesNotCorruptCommittedPanicResponse(t *testing.T) {
	exporter := captureHTTPSpans(t)
	handler := (&Server{}).observability(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("after commit")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(nethttp.MethodGet, "http://example.test/stream", nil))

	if response.Code != nethttp.StatusAccepted {
		t.Fatalf("response status = %d, want %d", response.Code, nethttp.StatusAccepted)
	}
	if got, want := response.Body.String(), "partial"; got != want {
		t.Fatalf("response body = %q, want %q", got, want)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Fatalf("span status = %v, want Error", span.Status.Code)
	}
	var status, bodySize int64
	for _, attr := range span.Attributes {
		switch string(attr.Key) {
		case "http.response.status_code":
			status = attr.Value.AsInt64()
		case "http.response.body.size":
			bodySize = attr.Value.AsInt64()
		}
	}
	if status != nethttp.StatusAccepted {
		t.Fatalf("span response status = %d, want %d", status, nethttp.StatusAccepted)
	}
	if bodySize != int64(len("partial")) {
		t.Fatalf("span body size = %d, want %d", bodySize, len("partial"))
	}
}

func TestRecordingResponseWriterCommitsFirstStatus(t *testing.T) {
	tests := []struct {
		name string
		act  func(*recordingResponseWriter) error
		want int
	}{
		{
			name: "explicit header",
			act: func(w *recordingResponseWriter) error {
				w.WriteHeader(nethttp.StatusCreated)
				w.WriteHeader(nethttp.StatusInternalServerError)
				return nil
			},
			want: nethttp.StatusCreated,
		},
		{
			name: "write commits ok",
			act: func(w *recordingResponseWriter) error {
				if _, err := w.Write([]byte("ok")); err != nil {
					return err
				}
				w.WriteHeader(nethttp.StatusInternalServerError)
				return nil
			},
			want: nethttp.StatusOK,
		},
		{
			name: "flush commits ok",
			act: func(w *recordingResponseWriter) error {
				w.Flush()
				w.WriteHeader(nethttp.StatusInternalServerError)
				return nil
			},
			want: nethttp.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			underlying := httptest.NewRecorder()
			writer := &recordingResponseWriter{ResponseWriter: underlying, status: nethttp.StatusOK}
			if err := test.act(writer); err != nil {
				t.Fatalf("act: %v", err)
			}
			if writer.status != test.want {
				t.Fatalf("recorded status = %d, want %d", writer.status, test.want)
			}
			if underlying.Code != test.want {
				t.Fatalf("response status = %d, want %d", underlying.Code, test.want)
			}
		})
	}
}

func captureHTTPSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previousTracer := tracer
	tracer = provider.Tracer("test/http")
	t.Cleanup(func() {
		tracer = previousTracer
		if err := provider.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})
	return exporter
}
