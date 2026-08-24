package runtimehost_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestPublicLargeToolResultIsDurableAndProgressivelyReadable(t *testing.T) {
	harness := newAcceptanceHarness(t)
	const sentinel = "LARGE_RESULT_SENTINEL_"
	body := strings.Repeat(sentinel, 4_000)
	if len(body) <= 64<<10 {
		t.Fatalf("large result fixture = %d bytes", len(body))
	}
	writeFixtureFile(t, filepath.Join(harness.workspace, "large-result.txt"), body)
	session := createRunSession(t, harness.runtime.baseURL, harness.namespace, "large-tool-result")
	ack, events := rpcRunStream[*protocol.StartRunResponse](
		t, harness.runtime.baseURL, "runs.start", protocol.StartRunRequest{
			SessionID: session.ID,
			Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "SCENARIO_LARGE_RESULT"}},
			Provider:  "openai-compatible", Model: "test-model",
		}, harness.meta, "large-tool-result-run", harness.namespace,
	)
	assertCompletedRunStream(t, ack.RunID, events)
	items := rpcCallWithMeta[*protocol.ListItemsResponse](
		t, harness.runtime.baseURL, "items.list", protocol.ListItemsRequest{
			Scope:     protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: ack.RunID},
			PageQuery: protocol.PageQuery{Limit: 100},
		}, harness.meta,
	)
	var readPreview, resultWindow string
	for _, item := range items.Data {
		if item.Type != protocol.ItemTypeToolCall || item.Status != protocol.ItemStatusCompleted || item.Tool == nil {
			continue
		}
		result, ok := item.Tool.Result.(string)
		if !ok {
			continue
		}
		switch item.Tool.Name {
		case "read":
			readPreview = result
		case "read_tool_result":
			resultWindow = result
		}
	}
	if len(readPreview) == 0 || len(readPreview) >= len(body) ||
		!strings.Contains(readPreview, "bytes omitted") || !strings.Contains(readPreview, `"result_id":"tr_`) {
		t.Fatalf("offloaded read preview = %d bytes: %.200q", len(readPreview), readPreview)
	}
	if len(resultWindow) == 0 || len(resultWindow) > 21_000 || !strings.Contains(resultWindow, sentinel) ||
		!strings.Contains(resultWindow, "continue with read_tool_result") {
		t.Fatalf("progressive result window = %d bytes: %.200q", len(resultWindow), resultWindow)
	}
	exported := rpcCall[*protocol.ExportSessionResponse](
		t, harness.runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
			SessionID: session.ID, Format: protocol.ExportFormatJSON,
		}, "", "",
	)
	if exported.Artifact == nil || len(exported.Artifact.ToolResults) != 1 ||
		len(exported.Artifact.ToolResults[0].Body) <= 64<<10 ||
		!strings.Contains(exported.Artifact.ToolResults[0].Body, sentinel) ||
		exported.Artifact.ToolResults[0].Preview != readPreview {
		t.Fatalf("large result artifact = %+v", exported.Artifact)
	}
	rpcCall[struct{}](
		t, harness.runtime.baseURL, "sessions.delete", protocol.DeleteSessionRequest{SessionID: session.ID},
		"delete-large-result-session", harness.namespace,
	)
	rpcCall[*protocol.ImportSessionResponse](
		t, harness.runtime.baseURL, "sessions.import", protocol.ImportSessionRequest{Artifact: *exported.Artifact},
		"import-large-result-session", harness.namespace,
	)
	restored := rpcCall[*protocol.ExportSessionResponse](
		t, harness.runtime.baseURL, "sessions.export", protocol.ExportSessionRequest{
			SessionID: session.ID, Format: protocol.ExportFormatJSON,
		}, "", "",
	)
	if restored.Artifact == nil || len(restored.Artifact.ToolResults) != 1 ||
		restored.Artifact.ToolResults[0].Body != exported.Artifact.ToolResults[0].Body ||
		restored.Artifact.ToolResults[0].Preview != readPreview {
		t.Fatal("large Tool result changed across export/import")
	}
}
