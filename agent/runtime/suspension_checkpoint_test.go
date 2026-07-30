package runtime

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

// TestSuspensionCheckpointNamesADeploymentOnlyWhenItHasOne pins what each
// checkpoint kind puts on the wire. validate rejects a nested_child checkpoint
// that carries any interaction state, so its encoded form must not name one
// either — and the tag enforcing that has to be omitzero, because omitempty is
// silently a no-op on a struct field and let the zero DeploymentRef ride along
// for as long as it was spelled that way.
func TestSuspensionCheckpointNamesADeploymentOnlyWhenItHasOne(t *testing.T) {
	nestedChild, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion:  suspensionCheckpointSchemaVersion,
		Kind:           suspensionCheckpointNestedChild,
		NestedChildren: []*nestedChildRelation{testNestedChildRelation("call-1", "child-1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"deployment", "owner", "checkpoint", "ready", "canceled_child"} {
		if _, named := checkpointKeys(t, nestedChild)[absent]; named {
			t.Errorf("nested child checkpoint names %q: %s", absent, nestedChild)
		}
	}

	// Marshaled directly: the assertion is about the tag, and a valid
	// interaction checkpoint would need a whole ToolLoop checkpoint to build.
	withDeployment, err := json.Marshal(suspensionCheckpoint{
		SchemaVersion: suspensionCheckpointSchemaVersion,
		Kind:          suspensionCheckpointInteraction,
		Owner:         "owner",
		Deployment:    core.DeploymentRef{Name: "demo", Digest: "digest"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, named := checkpointKeys(t, withDeployment)["deployment"]; !named {
		t.Fatalf("interaction checkpoint dropped its deployment: %s", withDeployment)
	}
}

func TestSuspensionCheckpointCanceledChildHasNoLiveRelation(t *testing.T) {
	canceled, err := encodeSuspensionCheckpoint(suspensionCheckpoint{
		SchemaVersion: suspensionCheckpointSchemaVersion,
		Kind:          suspensionCheckpointChildCanceled,
		CanceledChild: testNestedChildRelation("call-1", "child-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := checkpointKeys(t, canceled)
	if _, ok := keys["canceled_child"]; !ok {
		t.Fatalf("canceled checkpoint omitted historical child identity: %s", canceled)
	}
	for _, absent := range []string{"deployment", "owner", "checkpoint", "nested_children", "ready"} {
		if _, named := keys[absent]; named {
			t.Errorf("canceled checkpoint names live field %q: %s", absent, canceled)
		}
	}
}

func TestSuspensionCheckpointRejectsPreviousSchema(t *testing.T) {
	if _, err := parseSuspensionCheckpoint(json.RawMessage(
		`{"schema_version":2,"kind":"nested_child"}`,
	)); err == nil {
		t.Fatal("schema version 2 checkpoint was accepted")
	}
}

func checkpointKeys(t *testing.T, state json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(state, &keys); err != nil {
		t.Fatal(err)
	}
	return keys
}
