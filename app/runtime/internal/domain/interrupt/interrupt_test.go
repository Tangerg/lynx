package interrupt

import "testing"

func TestKindStringRoundTrip(t *testing.T) {
	for _, kind := range []Kind{Approval, Question} {
		parsed, ok := ParseKind(kind.String())
		if !ok || parsed != kind || !parsed.Valid() {
			t.Fatalf("ParseKind(%q) = %v, %t; want %v, true", kind.String(), parsed, ok, kind)
		}
	}
	if parsed, ok := ParseKind("unknown"); ok || parsed.Valid() {
		t.Fatalf("ParseKind(unknown) = %v, %t; want invalid", parsed, ok)
	}
}

func TestKeyIsStableAndKindSensitive(t *testing.T) {
	first := Key("approval", "shell", `{"command":"go test ./..."}`)
	if again := Key("approval", "shell", `{"command":"go test ./..."}`); again != first {
		t.Fatalf("Key() changed between equal inputs: %q != %q", first, again)
	}
	if question := Key("question", "shell", `{"command":"go test ./..."}`); question == first {
		t.Fatalf("Key() ignored interrupt kind: %q", first)
	}
}
