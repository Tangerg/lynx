package ollama_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

// jsonServer returns an httptest.Server that responds to every request
// with the given status code + body. Used for non-streaming endpoints
// (chat.Call, embedding.Call, image.Call, etc.).
//
// The optional inspect callback runs on every request, letting tests
// assert that the outgoing request shape (URL / method / headers /
// body) matches expectations.
func jsonServer(status int, body string, inspect ...func(r *http.Request)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, f := range inspect {
			f(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	return srv
}
