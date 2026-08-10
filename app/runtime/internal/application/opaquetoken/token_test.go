package opaquetoken

import "testing"

type testPayload struct {
	Value string `json:"value"`
}

func TestRoundTrip(t *testing.T) {
	token, err := Encode(testPayload{Value: "expected"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got testPayload
	if err := Decode(token, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Value != "expected" {
		t.Fatalf("value = %q, want expected", got.Value)
	}
}

func TestDecodeRejectsUnknownFieldsAndWrongShapes(t *testing.T) {
	for name, payload := range map[string]any{
		"unknown field": struct {
			Value string `json:"value"`
			Extra bool   `json:"extra"`
		}{Value: "expected", Extra: true},
		"wrong shape": []string{"expected"},
	} {
		t.Run(name, func(t *testing.T) {
			token, err := Encode(payload)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			var target testPayload
			if err := Decode(token, &target); err == nil {
				t.Fatal("decode accepted invalid payload")
			}
		})
	}
}
