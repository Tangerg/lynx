package transport

import (
	"strings"
	"testing"
)

func TestDecodeMessageRejectsDuplicateJSONMembersAtEveryObjectDepth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "envelope method",
			body: `{"jsonrpc":"2.0","id":"1","method":"sessions.list","method":"runs.list"}`,
			want: `duplicate JSON member "method"`,
		},
		{
			name: "escaped equivalent envelope method",
			body: `{"jsonrpc":"2.0","id":"1","method":"sessions.list","\u006dethod":"runs.list"}`,
			want: `duplicate JSON member "method"`,
		},
		{
			name: "request metadata",
			body: `{"jsonrpc":"2.0","id":"1","method":"sessions.list","params":{"_meta":{"protocolVersion":"2026-08-11","protocolVersion":"2026-08-10"}}}`,
			want: `duplicate JSON member "protocolVersion"`,
		},
		{
			name: "array member",
			body: `{"jsonrpc":"2.0","id":"1","method":"runs.start","params":{"input":[{"type":"text","text":"first","text":"second"}]}}`,
			want: `duplicate JSON member "text"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeMessage([]byte(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeMessage error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDecodeMessageRetainsSDKEnvelopeValidation(t *testing.T) {
	t.Parallel()

	message, err := DecodeMessage([]byte(
		`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`,
	))
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	request, ok := message.(*Request)
	if !ok || request.Method != "runtime.discover" {
		t.Fatalf("decoded message = %#v", message)
	}
}

func TestDecodeMessageRejectsAmbiguousOrOpenEnvelopeShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "request with result",
			body: `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{},"result":{}}`,
			want: `unknown JSON-RPC request member "result"`,
		},
		{
			name: "request with error",
			body: `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{},"error":{"code":-1,"message":"bad"}}`,
			want: `unknown JSON-RPC request member "error"`,
		},
		{
			name: "response with result and error",
			body: `{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-1,"message":"bad"}}`,
			want: "response contains both result and error",
		},
		{
			name: "response with params",
			body: `{"jsonrpc":"2.0","id":"1","result":{},"params":{}}`,
			want: `unknown JSON-RPC response member "params"`,
		},
		{
			name: "request extension",
			body: `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{},"extension":true}`,
			want: `unknown JSON-RPC request member "extension"`,
		},
		{
			name: "no discriminating member",
			body: `{"jsonrpc":"2.0","id":"1"}`,
			want: "contains neither method, result, nor error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeMessage([]byte(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeMessage error = %v, want %q", err, test.want)
			}
		})
	}

	message, err := DecodeMessage([]byte(
		`{"jsonrpc":"2.0","id":"1","result":{}}`,
	))
	if err != nil {
		t.Fatalf("DecodeMessage valid response: %v", err)
	}
	if _, ok := message.(*Response); !ok {
		t.Fatalf("decoded response = %#v", message)
	}
}

func TestDecodeMessageRejectsEmptyRequestMethod(t *testing.T) {
	t.Parallel()

	_, err := DecodeMessage([]byte(`{"jsonrpc":"2.0","id":"1","method":""}`))
	if err == nil || !strings.Contains(err.Error(), "request method is empty") {
		t.Fatalf("DecodeMessage error = %v, want empty method rejection", err)
	}
}

func TestDecodeMessageRequiresStringIDButAcceptsAnOmittedNotificationID(t *testing.T) {
	t.Parallel()

	invalidIDs := []string{"null", "42", "1.5", "1e100", "true", `[]`, `{}`}
	for _, id := range invalidIDs {
		_, err := DecodeMessage([]byte(
			`{"jsonrpc":"2.0","id":` + id + `,"method":"runtime.discover","params":{}}`,
		))
		if err == nil || !strings.Contains(err.Error(), "id must be a string") {
			t.Errorf("DecodeMessage id %s error = %v, want string-id rejection", id, err)
		}
	}

	message, err := DecodeMessage([]byte(
		`{"jsonrpc":"2.0","method":"test.notification","params":{}}`,
	))
	if err != nil {
		t.Fatalf("DecodeMessage notification: %v", err)
	}
	request, ok := message.(*Request)
	if !ok || request.IsCall() {
		t.Fatalf("decoded notification = %#v", message)
	}

	message, err = DecodeMessage([]byte(
		`{"jsonrpc":"2.0","id":"42","method":"runtime.discover","params":{}}`,
	))
	if err != nil {
		t.Fatalf("DecodeMessage string id: %v", err)
	}
	request, ok = message.(*Request)
	if !ok || !request.IsCall() {
		t.Fatalf("decoded call = %#v", message)
	}
}

func TestDecodeMessageRejectsExcessiveNesting(t *testing.T) {
	t.Parallel()

	const depth = 10_001
	encoded := []byte(strings.Repeat("[", depth) + strings.Repeat("]", depth))
	if _, err := DecodeMessage(encoded); err == nil {
		t.Fatal("DecodeMessage accepted JSON beyond the decoder nesting limit")
	}
}
