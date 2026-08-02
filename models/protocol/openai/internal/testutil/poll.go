package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
)

// Route names an HTTP method + path-substring pair plus its handler.
// The Contains field is matched against r.URL.Path with
// strings.Contains, so "/transcript" matches both "/v2/transcript"
// (the POST) and "/v2/transcript/job-1" (the GET poll). When Contains
// is empty the route matches every path — useful as a fallback.
type Route struct {
	Method   string
	Contains string
	Handle   http.HandlerFunc
}

// MuxServer returns an httptest.Server that dispatches incoming
// requests against `routes` in order — the first route whose
// method + suffix match wins. Useful for vendors that poll (upload
// → submit → poll), where one server has to answer three different
// requests over the lifetime of one Call.
//
// Unmatched requests return 404 with a hint so failures are
// debuggable.
func MuxServer(routes ...Route) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, route := range routes {
			if route.Method != "" && route.Method != r.Method {
				continue
			}
			if route.Contains != "" && !strings.Contains(r.URL.Path, route.Contains) {
				continue
			}
			route.Handle(w, r)
			return
		}
		http.Error(w, "testutil.MuxServer: no route matched "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

// PollCounter holds a goroutine-safe attempt counter. Polling vendors
// typically need to return "in-progress" for the first N polls then
// "completed" — bind a PollCounter to the GET handler to drive that.
type PollCounter struct {
	n atomic.Int32
}

// Inc returns the post-increment count.
func (p *PollCounter) Inc() int32 { return p.n.Add(1) }

// N returns the current count without incrementing.
func (p *PollCounter) N() int32 { return p.n.Load() }
