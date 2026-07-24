package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// codebase.* (API.md §7.10) — the @codebase semantic index for clients: the
// search surface (the Codebase workspace view), the status surface, and a manual
// reindex. The agent reaches the same index through the codebase_search tool.

// CodebaseSearch returns the chunks most similar to the query in the cwd's
// project (codebase.search), building/refreshing the index first.
func (s *Server) CodebaseSearch(ctx context.Context, in protocol.CodebaseSearchRequest) (*protocol.CodebaseSearchResult, error) {
	if !s.features.codebase {
		return nil, capabilityNotNegotiated("codebase.search")
	}
	if in.Query == "" {
		return nil, fmt.Errorf("%w: query is required", protocol.ErrInvalidParams)
	}
	hits, err := s.codebase.Search(ctx, in.Cwd, in.Query, in.Limit)
	if err != nil {
		return nil, mapCodebaseErr(err)
	}
	out := &protocol.CodebaseSearchResult{Hits: make([]protocol.CodebaseHit, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, protocol.CodebaseHit{
			Path:      h.Path,
			StartLine: h.StartLine,
			EndLine:   h.EndLine,
			Snippet:   h.Text,
			Score:     h.Score,
		})
	}
	return out, nil
}

// CodebaseStatus reports the cwd's index state (codebase.status).
func (s *Server) CodebaseStatus(ctx context.Context, in protocol.CodebaseStatusRequest) (*protocol.CodebaseStatus, error) {
	if !s.features.codebase {
		return nil, capabilityNotNegotiated("codebase.status")
	}
	st, err := s.codebase.Status(ctx, in.Cwd)
	if err != nil {
		return nil, mapCodebaseErr(err)
	}
	out, err := codebaseStatusToWire(st.Index)
	if err != nil {
		return nil, err
	}
	out.OperationID = st.OperationID
	return out, nil
}

// CodebaseReindex kicks a full rebuild in the background and returns immediately
// (codebase.reindex) — a big reindex can take seconds, so the status surface
// polls codebase.status for progress rather than blocking the call.
func (s *Server) CodebaseReindex(ctx context.Context, in protocol.CodebaseReindexRequest) (*protocol.CodebaseReindexResponse, error) {
	if !s.features.codebase {
		return nil, capabilityNotNegotiated("codebase.reindex")
	}
	operationID, err := s.codebase.StartReindex(ctx, in.Cwd)
	if err != nil {
		return nil, mapCodebaseErr(err)
	}
	return &protocol.CodebaseReindexResponse{OperationID: operationID}, nil
}

// mapCodebaseErr surfaces "no embedding model" as invalid_params with a fix-it
// message; other errors pass through (internal_error).
func mapCodebaseErr(err error) error {
	if errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		return fmt.Errorf("%w: no embedding model configured — set one in Settings (models.setEmbeddingRole)", protocol.ErrInvalidParams)
	}
	return wireWorkspaceError(err)
}

func codebaseStatusToWire(st codebaseindex.Status) (*protocol.CodebaseStatus, error) {
	state, ok := codebaseStateWire(st.State)
	if !ok {
		return nil, fmt.Errorf("codebase.status: unsupported state %q", st.State)
	}
	w := &protocol.CodebaseStatus{
		State:      state,
		ModelID:    st.ModelID,
		FileCount:  st.FileCount,
		ChunkCount: st.ChunkCount,
		Truncated:  st.Truncated,
	}
	if !st.IndexedAt.IsZero() {
		w.IndexedAt = st.IndexedAt.UTC().Format(time.RFC3339)
	}
	return w, nil
}

func codebaseStateWire(state codebaseindex.State) (protocol.CodebaseState, bool) {
	switch state {
	case codebaseindex.StateNone:
		return protocol.CodebaseStateNone, true
	case codebaseindex.StateIndexing:
		return protocol.CodebaseStateIndexing, true
	case codebaseindex.StateReady:
		return protocol.CodebaseStateReady, true
	case codebaseindex.StateError:
		return protocol.CodebaseStateError, true
	default:
		return "", false
	}
}
