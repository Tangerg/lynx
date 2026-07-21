package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func TestNestedChildStateOwnsRelationCopies(t *testing.T) {
	var state nestedChildState
	relation := testNestedChildRelation("call-1", "child-1")
	wantPrompt := bytes.Clone(relation.Prompt)
	wantSchema := bytes.Clone(relation.ResumeSchema)

	if err := state.stage("parent-1", relation); err != nil {
		t.Fatalf("stage: %v", err)
	}
	relation.Prompt[0] = '['
	relation.ResumeSchema[0] = '['

	first := state.relations()[relation.ToolCallID]
	if !bytes.Equal(first.Prompt, wantPrompt) || !bytes.Equal(first.ResumeSchema, wantSchema) {
		t.Fatalf("stored relation aliases caller: prompt=%s schema=%s", first.Prompt, first.ResumeSchema)
	}
	first.Prompt[0] = '['
	first.ResumeSchema[0] = '['

	second := state.relations()[relation.ToolCallID]
	if !bytes.Equal(second.Prompt, wantPrompt) || !bytes.Equal(second.ResumeSchema, wantSchema) {
		t.Fatalf("relation snapshot aliases state: prompt=%s schema=%s", second.Prompt, second.ResumeSchema)
	}
}

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

func TestNestedChildStateQueuesCleanupOnce(t *testing.T) {
	var state nestedChildState
	state.queueCleanup("child-1")
	state.queueCleanup("child-1")
	state.queueCleanup("child-2")

	cleanup := state.cleanupSnapshot()
	if len(cleanup) != 2 || cleanup[0] != "child-1" || cleanup[1] != "child-2" {
		t.Fatalf("cleanup = %v, want [child-1 child-2]", cleanup)
	}
	if cleanup := state.cleanupSnapshot(); len(cleanup) != 2 {
		t.Fatalf("unacknowledged cleanup = %v, want retained entries", cleanup)
	}
	state.acknowledgeCleanup([]string{"child-1", "child-2"})
	if cleanup := state.cleanupSnapshot(); cleanup != nil {
		t.Fatalf("acknowledged cleanup = %v, want nil", cleanup)
	}
}

func testNestedChildRelation(toolCallID, childID string) *nestedChildRelation {
	return &nestedChildRelation{
		SchemaVersion:       nestedChildRelationSchemaVersion,
		ToolCallID:          toolCallID,
		ChildID:             childID,
		Deployment:          core.DeploymentRef{Name: "child-agent", Digest: "digest"},
		SuspensionID:        "suspension-" + childID,
		SuspensionKind:      interaction.SuspensionHuman,
		SuspensionCreatedAt: time.Unix(1, 0).UTC(),
		Prompt:              json.RawMessage(`{"question":"continue?"}`),
		ResumeSchema:        json.RawMessage(`{"type":"boolean"}`),
		ToolName:            "delegate",
		ArgumentsDigest:     nestedArgumentsDigest(`{"task":"work"}`),
	}
}
