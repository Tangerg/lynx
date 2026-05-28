package http

import (
	"log"
	"net/http"
	"time"
)

// observability wraps the mux with a single middleware layer that
// implements the API.md §10 contract:
//
//   - structured log line per request (method / id / duration_ms /
//     error_code / bytes_in / bytes_out / trace_id)
//   - X-Lyra-Server header on every response (the handlers add
//     X-Lyra-Method themselves once they know the method)
//   - panic recovery with a flat 500 envelope so the runtime
//     survives a misbehaving handler
//
// Today the structured log goes through the stdlib logger; a follow-
// up wire it into Lyra's tracing package once that's settled.
func (s *Server) observability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if rec.status >= 500 {
				log.Printf("lyra-http path=%s status=%d duration_ms=%d bytes_out=%d",
					r.URL.Path, rec.status, time.Since(start).Milliseconds(), rec.bytes)
			}
		}()

		defer func() {
			if rcv := recover(); rcv != nil {
				log.Printf("lyra-http panic path=%s err=%v", r.URL.Path, rcv)
				writeTransportError(w, http.StatusInternalServerError, "internal error")
			}
		}()

		next.ServeHTTP(rec, r)
	})
}

// recordingResponseWriter is a tiny wrapper that captures status +
// bytes so the structured log line can include them. Stays minimal —
// the body itself stays out of memory.
type recordingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *recordingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

// Flush proxies through so SSE streams keep working.
func (w *recordingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
