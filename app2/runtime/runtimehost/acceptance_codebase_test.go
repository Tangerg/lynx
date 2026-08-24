package runtimehost_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestPublicCodebaseIndexBuildAndSearchSurface(t *testing.T) {
	harness := newAcceptanceHarness(t)
	baseURL, namespace, meta := harness.runtime.baseURL, harness.namespace, harness.meta
	workspace := protocol.WorkspaceRef{Path: harness.workspace}
	writeFixtureFile(t, filepath.Join(harness.workspace, "runtime.go"), `package runtime

// durableReplay returns the first committed command outcome.
func durableReplay() string { return "first-winner" }
`)
	writeFixtureFile(t, filepath.Join(harness.workspace, "README.md"), "# Runtime acceptance\n\nThe semantic index is workspace scoped.\n")

	status := rpcCallWithMeta[*protocol.CodebaseStatus](
		t, baseURL, "codebase.status", protocol.CodebaseStatusRequest{Workspace: workspace}, meta,
	)
	if status.State != protocol.CodebaseStateNone || status.OperationID != "" {
		t.Fatalf("initial codebase.status = %+v", status)
	}
	set := func(value string) *protocol.ProviderConfigChange {
		return &protocol.ProviderConfigChange{Type: protocol.ProviderConfigSet, Value: &value}
	}
	provider := rpcCall[*protocol.Provider](
		t, baseURL, "providers.update", protocol.UpdateProviderRequest{
			Provider: "openai", BaseURL: set(harness.model.URL), APIKey: set("test-key"),
		}, "configure-embedding-provider", namespace,
	)
	if provider.BaseURL != harness.model.URL || provider.APIKeyMasked == "" {
		t.Fatalf("configured embedding provider = %+v", provider)
	}
	role := rpcCall[*protocol.EmbeddingRole](
		t, baseURL, "models.setEmbeddingRole",
		protocol.EmbeddingRole{Provider: "openai", Model: "text-embedding-3-small"},
		"set-embedding-role", namespace,
	)
	if role.Provider != "openai" || role.Model != "text-embedding-3-small" {
		t.Fatalf("models.setEmbeddingRole = %+v", role)
	}
	started := rpcCall[*protocol.CodebaseReindexResponse](
		t, baseURL, "codebase.reindex", protocol.CodebaseReindexRequest{Workspace: workspace},
		"reindex-codebase", namespace,
	)
	if started.OperationID == "" {
		t.Fatal("codebase.reindex omitted operationId")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status = rpcCallWithMeta[*protocol.CodebaseStatus](
			t, baseURL, "codebase.status", protocol.CodebaseStatusRequest{Workspace: workspace}, meta,
		)
		if status.State == protocol.CodebaseStateReady || status.State == protocol.CodebaseStateError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if status.State != protocol.CodebaseStateReady || status.OperationID != "" ||
		status.ModelID != "openai/text-embedding-3-small" || status.FileCount < 2 ||
		status.ChunkCount < 2 || status.IndexedAt == "" {
		t.Fatalf("settled codebase.status = %+v", status)
	}
	result := rpcCallWithMeta[*protocol.CodebaseSearchResult](
		t, baseURL, "codebase.search", protocol.CodebaseSearchRequest{
			Workspace: workspace, Query: "durable command outcome", Limit: 5,
		}, meta,
	)
	if len(result.Hits) == 0 {
		t.Fatal("codebase.search returned no hits")
	}
	foundSource := false
	for _, hit := range result.Hits {
		if hit.Path == "runtime.go" && hit.StartLine > 0 && hit.EndLine >= hit.StartLine &&
			hit.Snippet != "" && hit.Score > 0 && hit.Score <= 1 {
			foundSource = true
		}
	}
	if !foundSource {
		t.Fatalf("codebase.search omitted source hit: %+v", result.Hits)
	}
}
