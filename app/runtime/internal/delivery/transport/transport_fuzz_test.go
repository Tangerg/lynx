package transport

import "testing"

func FuzzDecodeMessage(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"sessions.list","method":"runs.list"}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":null,"method":"runtime.discover","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":1.5,"method":"runtime.discover","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":1e100,"method":"runtime.discover","params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{},"result":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","result":{},"error":{"code":-1,"message":"bad"}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","result":{},"params":{}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","method":"runs.start","params":{"input":[{"type":"text","text":"first","text":"second"}]}}`))
	f.Add([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	f.Add([]byte(`{"jsonrpc":`))

	f.Fuzz(func(t *testing.T, encoded []byte) {
		message, err := DecodeMessage(encoded)
		if err != nil {
			return
		}
		reencoded, err := EncodeMessage(message)
		if err != nil {
			t.Fatalf("EncodeMessage after successful decode: %v", err)
		}
		if _, err := DecodeMessage(reencoded); err != nil {
			t.Fatalf("DecodeMessage rejected its canonical encoding %q: %v", reencoded, err)
		}
	})
}
