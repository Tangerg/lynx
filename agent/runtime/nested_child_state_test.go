package runtime

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func TestNestedChildStateRejectsConflictingOwnership(t *testing.T) {
	var state nestedChildState
	relation := testNestedChildRelation("call-1", "child-1")
	if err := state.stage("parent-1", relation); err != nil {
		t.Fatalf("stage first relation: %v", err)
	}
	if err := state.stage("parent-1", relation.clone()); err != nil {
		t.Fatalf("stage idempotent relation: %v", err)
	}

	conflict := testNestedChildRelation("call-2", relation.ChildID)
	if err := state.stage("parent-1", conflict); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("stage duplicate child error = %v, want ErrSuspensionConflict", err)
	}

	state.replacePending([]*nestedChildRelation{relation})
	changed := relation.clone()
	changed.ArgumentsDigest = nestedArgumentsDigest(`{"changed":true}`)
	if err := state.stage("parent-1", changed); !errors.Is(err, interaction.ErrSuspensionConflict) {
		t.Fatalf("stage changed invocation error = %v, want ErrSuspensionConflict", err)
	}
	if err := state.claim("parent-1", relation.ToolCallID, relation.ChildID); err != nil {
		t.Fatalf("claim pending child: %v", err)
	}
	if err := state.claim("parent-1", relation.ToolCallID, relation.ChildID); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("claim consumed child error = %v, want ErrSuspensionStale", err)
	}
}

func testNestedChildRelation(toolCallID, childID string) *nestedChildRelation {
	return &nestedChildRelation{
		SchemaVersion:   nestedChildRelationSchemaVersion,
		ToolCallID:      toolCallID,
		ChildID:         childID,
		Deployment:      core.DeploymentRef{Name: "child-agent", Digest: "digest"},
		ToolName:        "delegate",
		ArgumentsDigest: nestedArgumentsDigest(`{"task":"work"}`),
	}
}
