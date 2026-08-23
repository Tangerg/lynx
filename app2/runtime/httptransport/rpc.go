package httptransport

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/dispatch"
	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
)

const MaxRPCBodyBytes = 4 << 20

func (server *Server) serveRPC(response http.ResponseWriter, request *http.Request) {
	contentType := strings.TrimSpace(request.Header.Get("Content-Type"))
	if contentType != "" && !jsonMediaType(contentType) {
		writeProblem(response, http.StatusUnsupportedMediaType, "unsupported_media_type", "content-type must be application/json")
		return
	}
	if request.ContentLength > MaxRPCBodyBytes {
		writeProblem(response, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the transport limit")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxRPCBodyBytes+1))
	if err != nil {
		writeProblem(response, http.StatusBadRequest, "invalid_request", "request body could not be read")
		return
	}
	if len(body) > MaxRPCBodyBytes {
		writeProblem(response, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the transport limit")
		return
	}
	message, err := rpcwire.Decode(body)
	if err != nil {
		writeProblem(response, http.StatusBadRequest, "invalid_request", "invalid JSON-RPC message: "+err.Error())
		return
	}
	rpcRequest, ok := message.(*rpcwire.Request)
	if !ok {
		writeProblem(response, http.StatusBadRequest, "invalid_request", "POST /v2/rpc accepts only requests and notifications")
		return
	}
	response.Header().Set("X-Method", rpcRequest.Method)
	result := server.dispatcher.Dispatch(request.Context(), message, dispatch.Metadata{
		IdempotencyKey:       strings.TrimSpace(request.Header.Get("Idempotency-Key")),
		IdempotencyNamespace: strings.TrimSpace(request.Header.Get("Idempotency-Namespace")),
		AfterEventID:         strings.TrimSpace(request.Header.Get("Last-Event-Id")),
	})
	if result.Response == nil {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if result.Stream != nil && result.Response.Error == nil {
		server.serveStream(response, request, result.Response, result.Stream)
		return
	}
	encoded, err := rpcwire.Encode(result.Response)
	if err != nil {
		writeProblem(response, http.StatusInternalServerError, "response_encoding_failed", "the transport could not encode the RPC response")
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(encoded)
}

func (server *Server) requireToken(next http.Handler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if server.token == nil {
			next.ServeHTTP(response, request)
			return
		}
		token, err := server.token.Token(request.Context())
		if err != nil || token == "" {
			response.Header().Set("Cache-Control", "no-store")
			writeProblem(response, http.StatusServiceUnavailable, "authentication_unavailable", "the bearer credential is temporarily unavailable")
			return
		}
		if !validBearer(request.Header.Get("Authorization"), token) {
			response.Header().Set("WWW-Authenticate", "Bearer")
			response.Header().Set("Cache-Control", "no-store")
			writeProblem(response, http.StatusUnauthorized, "unauthorized", "a valid bearer credential is required")
			return
		}
		next.ServeHTTP(response, request)
	}
}

func validBearer(header, token string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func jsonMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

type transportProblem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	RequestID string `json:"requestId,omitempty"`
}

func writeProblem(response http.ResponseWriter, status int, problemType, detail string) {
	response.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(transportProblem{
		Type:      "urn:lyra:transport:" + problemType,
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		RequestID: response.Header().Get("Request-Id"),
	})
}
