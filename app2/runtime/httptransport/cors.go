package httptransport

import (
	"net/http"
	"slices"
	"strings"
)

var allowedRequestHeaders = []string{
	"authorization", "content-type", "idempotency-key", "idempotency-namespace",
	"last-event-id", "traceparent", "tracestate", "baggage",
}

func (server *Server) withCORS(next http.Handler) http.Handler {
	if len(server.origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		allowed := origin != "" && (slices.Contains(server.origins, origin) || slices.Contains(server.origins, "*"))
		response.Header().Add("Vary", "Origin")
		if allowed {
			response.Header().Set("Access-Control-Allow-Origin", origin)
			response.Header().Set("Access-Control-Allow-Credentials", "true")
			response.Header().Set("Access-Control-Expose-Headers", "Request-Id, X-Server, X-Method")
		}
		if request.Method != http.MethodOptions || request.Header.Get("Access-Control-Request-Method") == "" {
			next.ServeHTTP(response, request)
			return
		}
		response.Header().Add("Vary", "Access-Control-Request-Method")
		response.Header().Add("Vary", "Access-Control-Request-Headers")
		if !allowed || !allowedPreflight(request) {
			response.WriteHeader(http.StatusForbidden)
			return
		}
		response.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		response.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, Idempotency-Namespace, Last-Event-Id, traceparent, tracestate, baggage")
		response.Header().Set("Access-Control-Max-Age", "600")
		response.WriteHeader(http.StatusNoContent)
	})
}

func allowedPreflight(request *http.Request) bool {
	method := request.Header.Get("Access-Control-Request-Method")
	if method != http.MethodGet && method != http.MethodPost {
		return false
	}
	for _, header := range strings.Split(request.Header.Get("Access-Control-Request-Headers"), ",") {
		header = strings.ToLower(strings.TrimSpace(header))
		if header != "" && !slices.Contains(allowedRequestHeaders, header) {
			return false
		}
	}
	return true
}
