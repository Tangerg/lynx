package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/codebase"
)

const defaultCodebaseSearchLimit = 8

func (a *app) ShowCodebaseStatus() {
	if a.codebase == nil {
		a.message("this runtime composition has no codebase service")
		return
	}
	workspace := a.session.Workspace.Path
	a.runRuntimeReaderQuery("loading codebase index status", runtimeReaderCodebase,
		func(ctx context.Context) (readerDocument, error) {
			status, err := a.codebase.Status(ctx, workspace)
			if err != nil {
				return readerDocument{}, err
			}
			return codebaseStatusDocument(workspace, status), nil
		})
}

func codebaseStatusDocument(workspace string, status codebase.Status) readerDocument {
	lines := []string{
		"state      " + string(status.State),
		"model      " + fallback(status.ModelID, "not selected"),
		"files      " + strconv.Itoa(status.FileCount),
		"chunks     " + strconv.Itoa(status.ChunkCount),
		"truncated  " + strconv.FormatBool(status.Truncated),
	}
	if status.IndexedAt != nil {
		lines = append(lines, "indexed    "+status.IndexedAt.Format(time.RFC3339))
	}
	if status.OperationID != "" {
		lines = append(lines, "operation  "+status.OperationID)
	}
	return paragraphDocument("Codebase index", workspace, lines)
}

func (a *app) SearchCodebase(query string) error {
	if a.codebase == nil {
		return errors.New("this runtime composition has no codebase service")
	}
	request := codebase.Query{Workspace: a.session.Workspace.Path, Text: strings.TrimSpace(query), Limit: defaultCodebaseSearchLimit}
	if err := request.Validate(); err != nil {
		return errors.New("usage: /codebase-search <query>")
	}
	a.runRuntimeReaderQuery("searching semantic codebase", runtimeReaderCodebase,
		func(ctx context.Context) (readerDocument, error) {
			hits, err := a.codebase.Search(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			return codebaseHitsDocument(request, hits), nil
		})
	return nil
}

func codebaseHitsDocument(query codebase.Query, hits []codebase.Hit) readerDocument {
	if len(hits) == 0 {
		return paragraphDocument("Codebase search", query.Text, []string{"No semantic matches were found."})
	}
	sections := make([]ToolSection, 0, len(hits))
	for _, hit := range hits {
		sections = append(sections, ToolSection{
			Title: fmt.Sprintf("%s:%d-%d · %.3f", hit.Path, hit.StartLine, hit.EndLine, hit.Score),
			Style: toolSectionCode, Language: languageForPath(hit.Path), Text: hit.Snippet,
		})
	}
	return readerDocument{Title: "Codebase search", Detail: fmt.Sprintf("%d matches · %s", len(hits), query.Text), Sections: sections}
}

func (a *app) PrepareCodebaseReindex() error {
	if a.codebase == nil {
		return errors.New("this runtime composition has no codebase service")
	}
	workspace := a.session.Workspace.Path
	a.confirmAction("Reindex codebase", "Rebuild the semantic index for "+workspace+"?", "Start reindex", func() {
		a.reindexCodebase(workspace)
	})
	return nil
}

func (a *app) reindexCodebase(workspace string) {
	a.status.note("starting codebase reindex")
	if !runOperation(a, codebaseOperation, false,
		func(ctx context.Context) (codebaseReindexResult, error) {
			operation, err := a.codebase.Reindex(ctx, workspace)
			if err != nil {
				return codebaseReindexResult{}, err
			}
			status, err := a.codebase.Status(ctx, workspace)
			return codebaseReindexResult{operation: operation, status: status}, err
		},
		func(result codebaseReindexResult, err error) {
			if err != nil {
				a.message("start codebase reindex failed: " + err.Error())
				return
			}
			document := codebaseStatusDocument(workspace, result.status)
			document.Detail = "operation " + result.operation.ID + " · " + workspace
			a.setRuntimeReader(runtimeReaderCodebase)
			a.openReaderDocument(document)
			a.status.note("codebase reindex admitted · " + result.operation.ID)
		},
	) {
		a.message("another codebase operation is running")
	}
}

type codebaseReindexResult struct {
	operation codebase.ReindexOperation
	status    codebase.Status
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}
