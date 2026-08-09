package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestIdentityTypesRemainDistinctAndRoundTrip(t *testing.T) {
	processID, err := ParseProcessID("process:01J1-test")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(processID)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `"process:01J1-test"` {
		t.Fatalf("json.Marshal(ProcessID) = %s", got)
	}
	var decoded ProcessID
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != processID {
		t.Fatalf("decoded ProcessID = %q, want %q", decoded, processID)
	}
}

func TestIdentityRejectsEmptyUnsafeAndOversizedValues(t *testing.T) {
	values := []string{"", "contains space", "contains/slash", string(make([]byte, maxIdentityBytes+1))}
	for _, value := range values {
		if _, err := ParseSignalID(value); !errors.Is(err, ErrInvalidIdentity) {
			t.Fatalf("ParseSignalID(%q) error = %v, want ErrInvalidIdentity", value, err)
		}
	}
	if _, err := json.Marshal(ProcessID{}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("marshal zero ProcessID error = %v, want ErrInvalidIdentity", err)
	}
}
