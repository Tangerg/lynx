package offload

import (
	"errors"
	"testing"
	"time"
)

func TestParseID(t *testing.T) {
	id, err := ParseID("BLOB234")
	if err != nil || id != "BLOB234" {
		t.Fatalf("ParseID = (%q, %v)", id, err)
	}
	for _, raw := range []string{"", "A", "lowercase", "HAS-SPACE", "ABC018"} {
		if _, err := ParseID(raw); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("ParseID(%q) error = %v, want ErrInvalidID", raw, err)
		}
	}
}

func TestNewIDReturnsDistinctValidIdentities(t *testing.T) {
	first := NewID()
	second := NewID()
	if err := first.Validate(); err != nil {
		t.Fatalf("first ID: %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second ID: %v", err)
	}
	if first == second {
		t.Fatalf("NewID returned duplicate identity %q", first)
	}
}

func TestToolResultBlobValidate(t *testing.T) {
	blob := ToolResultBlob{
		ID: "BLOB234", SessionID: "ses_1", ItemID: "item_1", ToolName: "shell",
		Preview: "preview", Body: "body", CreatedAt: time.Now().UTC(),
	}
	if err := blob.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	blob.ItemID = ""
	if err := blob.Validate(); err == nil {
		t.Fatal("Validate accepted a blob without an item identity")
	}
}
