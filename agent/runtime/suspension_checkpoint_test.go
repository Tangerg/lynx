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
	for _, absent := range []string{"deployment", "owner", "checkpoint"} {
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

func checkpointKeys(t *testing.T, state json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(state, &keys); err != nil {
		t.Fatal(err)
	}
	return keys
}
