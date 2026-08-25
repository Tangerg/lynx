package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

type wireFixture struct {
	Message string `json:"message"`
}

func TestInputOwnsNormalizedJSON(t *testing.T) {
	source := json.RawMessage(` { "message": "hello" } `)
	input, err := ParseInput(source)
	if err != nil {
		t.Fatal(err)
	}
	source[3] = 'x'
	if got := string(input.JSON()); got != `{"message":"hello"}` {
		t.Fatalf("Input.JSON() = %s", got)
	}
	copyOfJSON := input.JSON()
	copyOfJSON[0] = '['
	if got := string(input.JSON()); got != `{"message":"hello"}` {
		t.Fatalf("Input shared returned bytes: %s", got)
	}
}

func TestInputRejectsMalformedAndMultipleValues(t *testing.T) {
	for _, data := range []json.RawMessage{nil, []byte(`{"message":`), []byte(`{} {}`)} {
		if _, err := ParseInput(data); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ParseInput(%q) error = %v, want ErrInvalidInput", data, err)
		}
	}
}

func TestTypedInputRejectsUnknownFields(t *testing.T) {
	input, err := ParseInput([]byte(`{"message":"hello","unknown":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := input.Decode[wireFixture](); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DecodeInput error = %v, want ErrInvalidInput", err)
	}
}

func TestOutputTypedRoundTrip(t *testing.T) {
	want := wireFixture{Message: "done"}
	output, err := EncodeOutput(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := output.Decode[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DecodeOutput() = %+v, want %+v", got, want)
	}
}

func TestWireZeroValuesAreInvalid(t *testing.T) {
	if (Input{}).Valid() || (Output{}).Valid() {
		t.Fatal("zero wire values reported valid")
	}
	if _, err := json.Marshal(Input{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("marshal Input error = %v, want ErrInvalidInput", err)
	}
	if _, err := json.Marshal(Output{}); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("marshal Output error = %v, want ErrInvalidOutput", err)
	}
}

func TestNormalizeJSONEnforcesNormalizedLimit(t *testing.T) {
	if _, err := normalizeJSON([]byte(`"<"`), 3); err == nil {
		t.Fatal("normalizeJSON accepted a value that exceeded the limit after normalization")
	}
}

func FuzzInputJSONRoundTrip(f *testing.F) {
	f.Add([]byte(`{"message":"hello"}`))
	f.Add([]byte(`[1,true,null]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		input, err := ParseInput(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Input
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if string(decoded.JSON()) != string(input.JSON()) {
			t.Fatalf("round trip = %s, want %s", decoded.JSON(), input.JSON())
		}
	})
}
