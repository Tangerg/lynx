package luma_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
)

// route names an HTTP method + path-substring pair plus its handler.
// The Contains field is matched against r.URL.Path with
// strings.Contains, so "/transcript" matches both "/v2/transcript"
// (the POST) and "/v2/transcript/job-1" (the GET poll). When Contains
// is empty the route matches every path — useful as a fallback.
type route struct {
	Method   string
	Contains string
	Handle   http.HandlerFunc
}

// muxServer returns an httptest.Server that dispatches incoming
// requests against `routes` in order — the first route whose
// method + suffix match wins. Useful for vendors that poll (upload
// → submit → poll), where one server has to answer three different
// requests over the lifetime of one Call.
//
// Unmatched requests return 404 with a hint so failures are
// debuggable.
func muxServer(routes ...route) *httptest.Server {
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
		http.Error(w, "muxServer: no route matched "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
}

// pollCounter holds a goroutine-safe attempt counter. Polling vendors
// typically need to return "in-progress" for the first N polls then
// "completed" — bind a pollCounter to the GET handler to drive that.
type pollCounter struct {
	n atomic.Int32
}

// Inc returns the post-increment count.
func (p *pollCounter) inc() int32 { return p.n.Add(1) }
