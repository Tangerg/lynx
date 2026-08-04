package server

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// TestArtifactVersionIsTheOneVNextFroze is half of contract §11.4 gate 15: the
// number, stated once.
//
// It is pinned to a literal on purpose. Comparing the stamped version to the
// constant proves only that one line reads another; what a version gate has to hold
// is that the document this build writes is the version the contract named. Bumping
// it is a breaking act, so it should cost a deliberate edit here.
func TestArtifactVersionIsTheOneVNextFroze(t *testing.T) {
	if protocol.SessionArtifactVersion != 12 {
		t.Fatalf("SessionArtifactVersion = %d; exclusive item timing requires artifact v12",
			protocol.SessionArtifactVersion)
	}
}

// TestArtifactV12RoundTripsEveryFieldItCarries is the rest of gate 15.
//
// The failure mode a version bump actually has is a field the encoder writes and
// the decoder drops — the archive still imports, still looks right, and the value is
// gone. A list of hand-picked assertions cannot catch that, because the field nobody
// remembered is the field nobody asserts. So this proves two things instead:
//
//   - the fixture reaches every field the document can carry, checked by walking the
//     shape rather than by trusting the fixture's author. A field that stops being
//     covered has to be named in [unreachableArtifactFields] with a reason;
//   - the archive survives the trip WHOLE — export, wipe, import, export again, and
//     the two documents must be identical byte for byte. Any field the decoder
//     forgets is missing from the second document.
func TestArtifactV12RoundTripsEveryFieldItCarries(t *testing.T) {
	s, rt := rollbackHarness(t)
	s.features.plan = true // this composition owns the key, so it may restore it
	ctx := t.Context()
	sessionID := seedMaximalSession(t, s, rt)

	exported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exported.Artifact == nil {
		t.Fatal("export produced no artifact")
	}
	if exported.Artifact.Version != protocol.SessionArtifactVersion {
		t.Fatalf("artifact version = %d, want %d", exported.Artifact.Version, protocol.SessionArtifactVersion)
	}
	assertArtifactFixtureIsComplete(t, *exported.Artifact)

	// Wipe the session so the import restores rather than merges.
	if err := rt.sess.Delete(ctx, sessionID); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := rt.Transcript().DeleteSession(ctx, sessionID); err != nil {
		t.Fatalf("delete transcript: %v", err)
	}
	_ = rt.TruncateMessages(ctx, sessionID, 0)

	if _, err := s.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *exported.Artifact}); err != nil {
		t.Fatalf("import: %v", err)
	}
	reexported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	before, after := encodeArtifact(t, *exported.Artifact), encodeArtifact(t, *reexported.Artifact)
	if before != after {
		t.Errorf("the artifact did not survive the round trip\n before: %s\n  after: %s", before, after)
	}
}

// TestExportPreservesRunTreeLineage proves an archive states child identity once:
// all three edges survive, while the protocol profile remains root-owned.
func TestExportPreservesRunTreeLineage(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "spawned", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	outcome := execution.OutcomeCompleted
	if err := rt.runs.Restore(ctx, transcript.Run{
		SessionID: ses.ID, ID: "run_root", State: execution.Completed,
		Outcome:      &outcome,
		Capabilities: execution.RunCapabilities{ChildRuns: true},
		CreatedAt:    time.Unix(1, 0).UTC(),
		FinishedAt:   time.Unix(1, 0).UTC(),
		UpdatedAt:    time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed root run: %v", err)
	}
	if err := rt.hist.AppendItem(ctx, transcript.Item{
		SessionID: ses.ID, RunID: "run_root", ID: "item_spawn",
		OccurredAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(1, 0).UTC(),
		Status: transcript.ItemCompleted,
		Kind:   transcript.ToolCall,
		Tool:   &transcript.ToolInvocation{Name: "delegate_task"},
	}); err != nil {
		t.Fatalf("seed spawning item: %v", err)
	}
	if err := rt.runs.Restore(ctx, transcript.Run{
		SessionID: ses.ID, ID: "run_child", SpawnedByItemID: "item_spawn",
		ParentRunID: "run_root", RootRunID: "run_root",
		State: execution.Completed, Outcome: &outcome,
		CreatedAt: time.Unix(2, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC(),
		UpdatedAt: time.Unix(2, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed child run: %v", err)
	}

	exported, err := s.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if exported.Artifact == nil || len(exported.Artifact.Runs) != 2 {
		t.Fatalf("artifact runs = %+v, want root and child", exported.Artifact)
	}
	child := exported.Artifact.Runs[1]
	if child.ID != "run_child" ||
		child.SpawnedByItemID != "item_spawn" ||
		child.ParentRunID != "run_root" ||
		child.RootRunID != "run_root" {
		t.Fatalf("exported child = %+v, want complete lineage", child)
	}
	if child.ProtocolProfile != nil {
		t.Fatalf("exported child protocol profile = %+v, want root-owned absence", child.ProtocolProfile)
	}
}

// TestImportRefusesAChildWhoseRootProfileDisallowsChildren proves import cannot
// bypass the same admission policy live execution obeys. The tree edges are
// structurally valid, but the root says the run was created under the Minimal
// Profile, so restoring the child would rewrite that frozen contract.
func TestImportRefusesAChildWhoseRootProfileDisallowsChildren(t *testing.T) {
	s, _ := rollbackHarness(t)
	at := time.Unix(1, 0).UTC()
	profile := protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{},
		InterruptTypes:   []protocol.InterruptType{},
	}
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{
			ID: "ses_tree", Title: "tree", Workspace: protocol.WorkspaceRef{Path: t.TempDir()}, CreatedAt: at, UpdatedAt: at,
		},
		Runs: []protocol.ArtifactRun{
			{
				ID: "run_root", SessionID: "ses_tree", ProtocolProfile: &profile,
				Outcome:   protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCompleted},
				CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			},
			{
				ID: "run_child", SessionID: "ses_tree",
				SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root",
				Outcome:   protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCompleted},
				CreatedAt: at, FinishedAt: at, UpdatedAt: at,
			},
		},
		Items: []protocol.ArtifactItem{{
			ID: "item_spawn", RunID: "run_root", Status: protocol.ItemStatusCompleted,
			StartedAt: at, FinishedAt: at, DurationMs: valuePtr(int64(0)),
			Type: protocol.ItemTypeToolCall,
			Tool: &protocol.ArtifactToolInvocation{Name: "delegate_task", Arguments: map[string]any{}},
		}},
	}

	_, err := s.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact})
	if !errors.Is(err, protocol.ErrInvalidParams) ||
		!strings.Contains(err.Error(), "run capabilities disallow child runs") {
		t.Fatalf("import error = %v, want invalid child/profile aggregate", err)
	}
}

func TestImportRefusesAnUnknownRunProtocolFeature(t *testing.T) {
	s, _ := rollbackHarness(t)
	profile := protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{"telepathy"},
		InterruptTypes:   []protocol.InterruptType{},
	}
	artifact := protocol.SessionArtifact{
		Version: protocol.SessionArtifactVersion,
		Session: protocol.ArtifactSession{
			ID: "ses_unknown_profile", Title: "profile", Workspace: protocol.WorkspaceRef{Path: t.TempDir()},
		},
		Runs: []protocol.ArtifactRun{{
			ID: "run_root", SessionID: "ses_unknown_profile", ProtocolProfile: &profile,
			Outcome: protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCompleted},
		}},
	}

	_, err := s.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact})
	if !errors.Is(err, protocol.ErrInvalidParams) ||
		!strings.Contains(err.Error(), `unknown value "telepathy"`) {
		t.Fatalf("import error = %v, want invalid required feature", err)
	}
}

func encodeArtifact(t *testing.T, artifact protocol.SessionArtifact) string {
	t.Helper()
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	return string(encoded)
}

// unreachableArtifactFields are the fields a maximal export cannot populate, each
// with the reason it cannot.
//
// It is a closed list, and a field that becomes reachable has to leave it: "the
// fixture quietly stopped exercising this" and "this cannot be exercised" have to
// look different, or the coverage check decays into a list of excuses.
var unreachableArtifactFields = map[string]string{}

// assertArtifactFixtureIsComplete reports any field of the artifact document the
// fixture left at its zero value, and any entry in the exception list that turned
// out to be reachable after all.
func assertArtifactFixtureIsComplete(t *testing.T, artifact protocol.SessionArtifact) {
	t.Helper()

	declared := map[string]bool{}
	walkArtifactShape(reflect.TypeFor[protocol.SessionArtifact](), declared, map[reflect.Type]bool{})
	populated := map[string]bool{}
	markPopulatedFields(reflect.ValueOf(artifact), populated)

	for _, field := range slices.Sorted(maps.Keys(declared)) {
		reason, excused := unreachableArtifactFields[field]
		switch {
		case populated[field] && excused:
			t.Errorf("%s is populated AND excused (%q) — one of the two is wrong", field, reason)
		case !populated[field] && !excused:
			t.Errorf("%s is part of the v9 document and the round-trip fixture never sets it, "+
				"so nothing proves it survives an import", field)
		}
	}
	for field := range unreachableArtifactFields {
		if !declared[field] {
			t.Errorf("%s is excused from the round-trip fixture and no longer exists", field)
		}
	}
}

// walkArtifactShape collects every "Type.Field" the artifact document can carry.
// It descends through pointers, slices and maps, and stops at anything that is not
// a struct — time.Time and json.RawMessage contribute no fields of their own, and
// an `any` is opaque by definition.
func walkArtifactShape(shape reflect.Type, into map[string]bool, seen map[reflect.Type]bool) {
	switch shape.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		walkArtifactShape(shape.Elem(), into, seen)
	case reflect.Struct:
		if seen[shape] {
			return
		}
		seen[shape] = true
		for index := range shape.NumField() {
			field := shape.Field(index)
			if !field.IsExported() {
				continue
			}
			into[shape.Name()+"."+field.Name] = true
			walkArtifactShape(field.Type, into, seen)
		}
	}
}

// markPopulatedFields records every field the value actually sets. A zero field is
// not descended into: there is nothing inside it to have been carried.
func markPopulatedFields(value reflect.Value, into map[string]bool) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			markPopulatedFields(value.Elem(), into)
		}
	case reflect.Slice, reflect.Array:
		for index := range value.Len() {
			markPopulatedFields(value.Index(index), into)
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			markPopulatedFields(value.MapIndex(key), into)
		}
	case reflect.Struct:
		shape := value.Type()
		for index := range shape.NumField() {
			field := shape.Field(index)
			if !field.IsExported() || !carriesAValue(value.Field(index)) {
				continue
			}
			into[shape.Name()+"."+field.Name] = true
			markPopulatedFields(value.Field(index), into)
		}
	}
}

// carriesAValue reports whether a field actually carries something. An empty
// collection does not: the wire requires some arrays to be present, and counting a
// `[]` as coverage would let a field claim it round-trips when the only thing proven
// is that both sides write nothing.
func carriesAValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.String:
		return value.Len() != 0
	default:
		return !value.IsZero()
	}
}

// seedMaximalSession writes a session that reaches every corner of the v9 document:
// two runs (one completed with full accounting, one failed with its problem), one
// item per transcript kind, an offloaded tool body, and a Plan.
func seedMaximalSession(t *testing.T, s *Server, rt *stubRuntime) string {
	t.Helper()
	ctx := t.Context()
	const sessionID = "ses_maximal"
	// The canonical spelling: an import resolves the cwd, so seeding the raw temp
	// path would make the two exports differ by macOS's /private prefix rather than
	// by anything the archive carries.
	cwd := canonicalWorkspacePath(t, t.TempDir())
	if err := rt.sess.Restore(ctx, session.Session{
		ID: sessionID, Title: "Everything", Cwd: cwd, Model: "claude-opus-5",
		StartedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(9, 0).UTC(),
		Favorite: true,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := rt.SeedHistory(ctx, sessionID, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("do everything")),
		chat.NewAssistantMessage(chat.NewTextPart("done")),
	}); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	seedCompletedRun(t, rt, sessionID)
	seedFailedRun(t, rt, sessionID)
	seedEveryItemKind(t, rt, sessionID)
	seedChildRun(t, rt, sessionID)
	seedOffloadedToolResult(t, rt, sessionID)

	if err := rt.plan.Replace(ctx, sessionID, []plan.Step{
		{Description: "carry every field", Status: plan.StatusInProgress},
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return sessionID
}

func seedCompletedRun(t *testing.T, rt *stubRuntime, sessionID string) {
	t.Helper()
	cost := 1.25
	outcome := execution.OutcomeCompleted
	selection, err := modelref.New("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("model selection: %v", err)
	}
	if err := rt.runs.Restore(t.Context(), transcript.Run{
		SessionID: sessionID, ID: "run_done", State: execution.Completed,
		ModelSelection: selection, Outcome: &outcome,
		Limits: execution.RunLimits{MaxTotalTokens: 32_768, MaxSteps: 12, MaxBudgetUSD: 3.5},
		Metrics: transcript.RunMetrics{
			Usage: &transcript.Usage{
				ModelUsage: transcript.ModelUsage{
					InputTokens: 100, OutputTokens: 20, CacheReadTokens: 5,
					CacheWriteTokens: 3, ReasoningTokens: 7, CostUSD: &cost,
				},
				ByModel: map[string]transcript.ModelUsage{
					"claude-opus-5": {
						InputTokens: 100, OutputTokens: 20, CacheReadTokens: 5,
						CacheWriteTokens: 3, ReasoningTokens: 7, CostUSD: &cost,
					},
				},
			},
			Steps:          2,
			ActiveDuration: 1500 * time.Millisecond,
		},
		Capabilities: execution.RunCapabilities{
			ChildRuns:      true,
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		},
		CreatedAt: time.Unix(2, 0).UTC(), FinishedAt: time.Unix(3, 0).UTC(),
		UpdatedAt: time.Unix(3, 0).UTC(), MessageMark: 1,
	}); err != nil {
		t.Fatalf("seed completed run: %v", err)
	}
}

func seedChildRun(t *testing.T, rt *stubRuntime, sessionID string) {
	t.Helper()
	outcome := execution.OutcomeCompleted
	selection, err := modelref.New("openai", "gpt-child")
	if err != nil {
		t.Fatalf("child model selection: %v", err)
	}
	if err := rt.runs.Restore(t.Context(), transcript.Run{
		SessionID: sessionID, ID: "run_child", State: execution.Completed,
		SpawnedByItemID: "item_tool", ParentRunID: "run_done", RootRunID: "run_done",
		ModelSelection: selection,
		Outcome:        &outcome,
		CreatedAt:      time.Unix(7, 0).UTC(),
		FinishedAt:     time.Unix(8, 0).UTC(),
		UpdatedAt:      time.Unix(8, 0).UTC(),
		MessageMark:    1,
	}); err != nil {
		t.Fatalf("seed child run: %v", err)
	}
}

func seedFailedRun(t *testing.T, rt *stubRuntime, sessionID string) {
	t.Helper()
	outcome := execution.OutcomeError
	if err := rt.runs.Restore(t.Context(), transcript.Run{
		SessionID: sessionID, ID: "run_failed", State: execution.Failed,
		Outcome: &outcome,
		Detail:  "the provider gave up",
		Error: &transcript.Problem{
			Kind: transcript.RateLimitedProblem, Scope: transcript.RunProblem,
			Detail: "slow down", DocURL: "https://example.invalid/rate-limits",
			RetryAfterSeconds: 30,
		},
		Metrics: transcript.RunMetrics{Steps: 1, ActiveDuration: 500 * time.Millisecond},
		Capabilities: execution.RunCapabilities{
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		},
		CreatedAt: time.Unix(4, 0).UTC(), FinishedAt: time.Unix(5, 0).UTC(),
		UpdatedAt: time.Unix(5, 0).UTC(), MessageMark: 2,
	}); err != nil {
		t.Fatalf("seed failed run: %v", err)
	}
}

// seedEveryItemKind writes one item per transcript kind, spreading the optional
// members across them: an ArtifactItem is a union in a flat struct, so no single
// item can populate all of it.
func seedEveryItemKind(t *testing.T, rt *stubRuntime, sessionID string) {
	t.Helper()
	arguments, err := tool.ArgumentsFromMap(map[string]any{"command": "ls", "description": "List workspace files"})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	result, err := tool.NewResult(map[string]any{"stdout": "total 0\n"})
	if err != nil {
		t.Fatalf("tool result: %v", err)
	}
	items := []transcript.Item{
		{
			ID: "item_user", RunID: "run_done", Kind: transcript.UserMessage,
			Status: transcript.ItemCompleted, OccurredAt: time.Unix(2, 0).UTC(),
			Content: []transcript.ContentBlock{
				{Kind: transcript.TextContent, Text: "do everything"},
				{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("hello")},
			},
		},
		{
			ID: "item_agent", RunID: "run_done", Kind: transcript.AgentMessage,
			Status: transcript.ItemCompleted, OccurredAt: time.Unix(3, 0).UTC(),
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "on it"}},
		},
		{
			ID: "item_reasoning", RunID: "run_done", Kind: transcript.Reasoning,
			Status: transcript.ItemIncomplete, OccurredAt: time.Unix(4, 0).UTC(),
			Text: "thinking about it", Redacted: true,
		},
		{
			ID: "item_question", RunID: "run_done", Kind: transcript.QuestionItem,
			Status: transcript.ItemCompleted, OccurredAt: time.Unix(6, 0).UTC(),
			Question: &transcript.Question{
				Fields: []transcript.QuestionField{{
					Prompt: "Which route?", Header: "Pick one",
					Kind: transcript.QuestionChoice, Multiple: true, AllowCustom: true,
					Options: []transcript.QuestionOption{
						{Label: "left", Description: "the short way", Preview: "◀"},
						{Label: "right", Description: "the scenic way", Preview: "▶"},
					},
				}},
			},
		},
		{
			ID: "item_tool", RunID: "run_done", Kind: transcript.ToolCall,
			Status: transcript.ItemCompleted, OccurredAt: time.Unix(7, 0).UTC(),
			FinishedAt:  time.UnixMilli(7250).UTC(),
			SafetyClass: tool.SafetyClassExec,
			Tool:        &transcript.ToolInvocation{Name: "shell", Arguments: arguments, Result: &result},
		},
		{
			ID: "item_failed", RunID: "run_failed", Kind: transcript.ToolCall,
			Status: transcript.ItemIncomplete, OccurredAt: time.Unix(8, 0).UTC(),
			FinishedAt: time.UnixMilli(8500).UTC(),
			Error: &transcript.Problem{
				Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem,
				Detail: "exit 1", DocURL: "https://example.invalid/tools",
			},
			Tool: &transcript.ToolInvocation{Name: "shell", Arguments: arguments},
		},
		{
			ID: "item_compaction", RunID: "run_failed", Kind: transcript.Compaction,
			Status: transcript.ItemCompleted, OccurredAt: time.Unix(9, 0).UTC(),
			Summary: "folded the earlier turns", DroppedMessages: 4,
		},
	}
	for _, item := range items {
		item.SessionID = sessionID
		if err := rt.hist.AppendItem(t.Context(), item); err != nil {
			t.Fatalf("seed item %s: %v", item.ID, err)
		}
	}
}

// seedOffloadedToolResult stages a body large enough to live outside the item and
// binds it, which is the only way ArtifactToolResult gets an entry.
func seedOffloadedToolResult(t *testing.T, rt *stubRuntime, sessionID string) {
	t.Helper()
	ctx := t.Context()
	body := strings.Repeat("offloaded-", 200)
	id := offload.NewID()
	if err := rt.toolResults.Stage(ctx, offload.ToolResultStage{
		ID: id, SessionID: sessionID, ToolName: "vendor_tool", Body: body,
	}); err != nil {
		t.Fatalf("stage tool result: %v", err)
	}
	preview := "offloaded preview " + id.String()
	previewValue := tool.StringResult(preview)
	if err := rt.hist.AppendItem(ctx, transcript.Item{
		SessionID: sessionID, RunID: "run_done", ID: "item_offload",
		Kind: transcript.ToolCall, Status: transcript.ItemCompleted,
		OccurredAt: time.Unix(10, 0).UTC(), FinishedAt: time.Unix(11, 0).UTC(),
		Tool: &transcript.ToolInvocation{
			Name: "vendor_tool", Result: &previewValue, Offload: &offload.Ref{ID: id},
		},
	}); err != nil {
		t.Fatalf("seed offloaded item: %v", err)
	}
	if err := rt.toolResults.Bind(ctx, sessionID, "item_offload", preview, offload.Ref{ID: id}); err != nil {
		t.Fatalf("bind tool result: %v", err)
	}
}
