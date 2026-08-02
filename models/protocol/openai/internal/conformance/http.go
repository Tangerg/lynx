package conformance

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewBlockingServer starts a mock request that remains in flight until the
// client releases its context. writeInitial may emit and flush one valid stream
// event before Started closes; pass nil for a blocking Call.
func NewBlockingServer(t *testing.T, writeInitial func(http.ResponseWriter)) (*httptest.Server, Lifecycle) {
	t.Helper()
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(stopped)
		if writeInitial != nil {
			writeInitial(writer)
		} else {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			writer.(http.Flusher).Flush()
		}
		close(started)
		<-request.Context().Done()
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	return server, Lifecycle{Started: started, Stopped: stopped}
}
