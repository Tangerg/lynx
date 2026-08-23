package rpcwire_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/rpcwire"
)

func TestDecodeAcceptsOnlyClosedUnambiguousMessages(t *testing.T) {
	t.Parallel()

	valid := []string{
		`{"jsonrpc":"2.0","id":"req_1","method":"runtime.discover","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications.client.ready","params":{}}`,
		`{"jsonrpc":"2.0","id":"req_1","result":{}}`,
		`{"jsonrpc":"2.0","id":"req_1","error":{"code":-32603,"message":"internal_error"}}`,
	}
	for _, encoded := range valid {
		if _, err := rpcwire.Decode([]byte(encoded)); err != nil {
			t.Errorf("Decode(%s) error = %v", encoded, err)
		}
	}

	invalid := []struct {
		name string
		body string
		want string
	}{
		{"duplicate envelope", `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","method":"runs.list"}`, "duplicate"},
		{"duplicate nested", `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{"_meta":{"protocolVersion":"a","protocolVersion":"b"}}}`, "duplicate"},
		{"numeric id", `{"jsonrpc":"2.0","id":1,"method":"runtime.discover"}`, "id must be a string"},
		{"null id", `{"jsonrpc":"2.0","id":null,"method":"runtime.discover"}`, "id must be a string"},
		{"open request", `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","extension":true}`, "unknown JSON-RPC request member"},
		{"mixed request", `{"jsonrpc":"2.0","id":"1","method":"runtime.discover","result":{}}`, "unknown JSON-RPC request member"},
		{"mixed response", `{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-1,"message":"bad"}}`, "both result and error"},
		{"empty method", `{"jsonrpc":"2.0","id":"1","method":""}`, "method is empty"},
		{"multiple values", `{"jsonrpc":"2.0","id":"1","result":{}} {}`, "more than one"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := rpcwire.Decode([]byte(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want text %q", err, test.want)
			}
		})
	}
}

func FuzzDecodeRoundTrip(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":null,"method":"runtime.discover"}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"x.y","method":"a.b"}`))
	f.Fuzz(func(t *testing.T, encoded []byte) {
		message, err := rpcwire.Decode(encoded)
		if err != nil {
			return
		}
		canonical, err := rpcwire.Encode(message)
		if err != nil {
			t.Fatalf("Encode() after Decode() error = %v", err)
		}
		if _, err := rpcwire.Decode(canonical); err != nil {
			t.Fatalf("Decode(Encode(message)) error = %v; bytes = %q", err, canonical)
		}
	})
}
