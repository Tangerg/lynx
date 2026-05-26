package agui

import (
	"encoding/json"
	"net/http"
)

// REST endpoints — read-only artifact APIs the frontend's react-query hooks
// hit on mount. Co-located with the SSE handler so one Server hosts both.

func (s *Server) registerREST(mux *http.ServeMux) {
	// Sessions get a per-request handler because their timestamps are
	// relative to "now" — see artifacts.go::makeSessions.
	mux.HandleFunc("/sessions", jsonFuncHandler(makeSessions))
	mux.HandleFunc("/projects", jsonHandler(projects))
	mux.HandleFunc("/files-changed", jsonHandler(filesChanged))
	mux.HandleFunc("/diff", jsonHandler(diff))
	mux.HandleFunc("/terminal", jsonHandler(termLines))
	mux.HandleFunc("/grep", jsonHandler(grep))
	mux.HandleFunc("/file-head", jsonHandler(fileHead))
	mux.HandleFunc("/mcp-servers", jsonHandler(mcpServers))

	// Sideloaded plugin manifest + static assets.
	mux.HandleFunc("/plugins", s.handlePluginsList)
	mux.HandleFunc("/plugins/", s.handlePluginAsset)
}

// jsonHandler returns an HTTP handler that serves the given value as JSON.
// GET only — anything else is 405.
func jsonHandler(v any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(v)
	}
}

// jsonFuncHandler is `jsonHandler` for values that need to be computed
// per request (e.g. timestamps relative to the wall clock).
func jsonFuncHandler[T any](build func() T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(build())
	}
}
