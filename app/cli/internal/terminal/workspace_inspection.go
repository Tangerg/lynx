package terminal

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

const workspaceWatchID = "lyra-active-workspace"

type workspaceReaderMode uint8

const (
	workspaceReaderNone workspaceReaderMode = iota
	workspaceReaderChanges
)

func (a *app) ShowWorkspaces() {
	a.runWorkspaceQuery("loading workspaces",
		func(ctx context.Context) (readerDocument, error) {
			values, err := a.workspaces.List(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(values))
			for _, value := range values {
				label := value.Name + "  " + value.Workspace.Path
				if value.Sessions > 0 {
					label += fmt.Sprintf("  · %d sessions", value.Sessions)
				}
				if !value.Workspace.IsAvailable() {
					label += "  · missing"
				}
				lines = append(lines, label)
			}
			return paragraphDocument("Runtime workspaces", fmt.Sprintf("%d known", len(values)), lines), nil
		}, workspaceReaderNone)
}

func (a *app) ShowWorkspaceChanges() {
	path := a.session.Workspace
	a.runWorkspaceQuery("loading workspace changes",
		func(ctx context.Context) (readerDocument, error) {
			changes, err := a.workspaces.Changes(ctx, path)
			if err != nil {
				return readerDocument{}, err
			}
			return workspaceChangesDocument(path, changes), nil
		}, workspaceReaderChanges)
}

func (a *app) ShowWorkspaceDiff(path string) {
	request := workspace.DiffRequest{
		Workspace: a.session.Workspace, Path: strings.TrimSpace(path),
		Mode: workspace.DiffModeWorktree, Format: workspace.DiffFormatRaw,
	}
	a.runWorkspaceQuery("loading workspace diff",
		func(ctx context.Context) (readerDocument, error) {
			diff, err := a.workspaces.Diff(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			text := diff.Text()
			if strings.TrimSpace(text) == "" {
				text = "No workspace differences."
			}
			detail := "worktree"
			if request.Path != "" {
				detail += " · " + request.Path
			}
			if diff.Truncated {
				detail += " · truncated"
			}
			return readerDocument{Title: "Workspace diff", Detail: detail, Sections: []ToolSection{{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: text}}}, nil
		}, workspaceReaderNone)
}

func (a *app) PreviewWorkspaceFile(path string) {
	request := workspace.HeadRequest{Workspace: a.session.Workspace, Path: strings.TrimSpace(path), Lines: 80}
	a.runWorkspaceQuery("loading file preview",
		func(ctx context.Context) (readerDocument, error) {
			head, err := a.workspaces.Head(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(head.Lines))
			for _, line := range head.Lines {
				lines = append(lines, line.Text)
			}
			return codeDocument("File preview", head.Path, strings.Join(lines, "\n"), head.Path, true), nil
		}, workspaceReaderNone)
}

func (a *app) SearchWorkspace(query string) {
	request := workspace.SearchRequest{Workspace: a.session.Workspace, Query: strings.TrimSpace(query), Limit: 200}
	a.runWorkspaceQuery("searching workspace",
		func(ctx context.Context) (readerDocument, error) {
			result, err := a.workspaces.Search(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(result.Matches))
			for _, match := range result.Matches {
				lines = append(lines, fmt.Sprintf("%s:%d  %s", match.Path, match.Line, match.Text))
			}
			detail := fmt.Sprintf("%d/%d matches", len(result.Matches), result.Total)
			return paragraphDocument("Workspace search · "+request.Query, detail, lines), nil
		}, workspaceReaderNone)
}

func (a *app) BrowseWorkspace(path string) {
	request := workspace.FilesRequest{
		Workspace: a.session.Workspace, Path: strings.TrimSpace(path), Limit: 500,
	}
	a.runWorkspaceQuery("browsing workspace",
		func(ctx context.Context) (readerDocument, error) {
			page, err := a.workspaces.Files(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(page.Entries)+1)
			for _, entry := range page.Entries {
				kind := string(entry.Type)
				if entry.Type == workspace.FileEntryDirectory {
					kind = "dir"
				}
				lines = append(lines, fmt.Sprintf("%-7s %s", kind, entry.Path))
			}
			detail := fmt.Sprintf("%d entries", len(page.Entries))
			if page.NextCursor != "" {
				detail += " · more available"
			}
			title := "Workspace files"
			if request.Path != "" {
				title += " · " + request.Path
			}
			return codeDocument(title, detail, strings.Join(lines, "\n"), "text", false), nil
		}, workspaceReaderNone)
}

func (a *app) ReadWorkspaceFile(path string) {
	request := workspace.ReadRequest{Workspace: a.session.Workspace, Path: strings.TrimSpace(path), MaxBytes: 2 << 20}
	a.runWorkspaceQuery("reading workspace file",
		func(ctx context.Context) (readerDocument, error) {
			content, err := a.workspaces.Read(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			detail := content.Window()
			if content.Truncated {
				detail += " · truncated"
			}
			return codeDocument("Workspace file", detail, content.Content, content.Path, true), nil
		}, workspaceReaderNone)
}

func (a *app) runWorkspaceQuery(status string, query func(context.Context) (readerDocument, error), mode workspaceReaderMode) {
	a.status.note(status)
	runOperation(a, workspaceQueryOperation, true, query, func(document readerDocument, err error) {
		if err != nil {
			a.message("workspace: " + err.Error())
			return
		}
		a.workspaceReader = mode
		a.openReaderDocument(document)
		a.status.note(strings.ToLower(document.Title))
	})
}

func paragraphDocument(title, detail string, lines []string) readerDocument {
	text := strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" {
		text = "No results."
	}
	return readerDocument{Title: title, Detail: detail, Sections: []ToolSection{{Title: "Results", Style: toolSectionParagraph, Text: text}}}
}

func codeDocument(title, detail, text, pathOrLanguage string, lineNumbers bool) readerDocument {
	language := pathOrLanguage
	if strings.Contains(pathOrLanguage, ".") || strings.ContainsRune(pathOrLanguage, filepath.Separator) {
		language = languageForPath(pathOrLanguage)
	}
	return readerDocument{Title: title, Detail: detail, Sections: []ToolSection{{Title: "Content", Style: toolSectionCode, Language: language, Text: text, LineNumbers: lineNumbers}}}
}

func workspaceChangesDocument(path string, changes []workspace.Change) readerDocument {
	ordered := append([]workspace.Change(nil), changes...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Path < ordered[right].Path })
	lines := make([]string, 0, len(ordered))
	for _, change := range ordered {
		pathLabel := change.Path
		if change.PreviousPath != "" {
			pathLabel = change.PreviousPath + " → " + change.Path
		}
		stat := change.Stat()
		if stat != "" {
			stat = "  " + stat
		}
		lines = append(lines, fmt.Sprintf("%-10s %s%s", change.Status, pathLabel, stat))
	}
	return paragraphDocument("Workspace changes", fmt.Sprintf("%d files · %s", len(changes), path), lines)
}

func (a *app) followWorkspaceChanges() {
	a.operations.Cancel(workspaceChangesOperation)
	if a.workspaces == nil {
		return
	}
	workspacePath := a.session.Workspace
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(workspaceChangesOperation, true, func(ctx context.Context, lease operationLease) {
		monitor := workspaceChangeMonitor{
			workspace: workspacePath, repository: a.workspaces, source: a.changes,
			apply: func(changes []workspace.Change) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.Workspace != workspacePath {
						return
					}
					a.applyWorkspaceChanges(changes)
				})
			},
		}
		monitor.run(ctx)
	})
}

func (a *app) applyWorkspaceChanges(changes []workspace.Change) {
	a.header.SetWorkspaceChanges(len(changes))
	if a.workspaceReader == workspaceReaderChanges {
		follow := a.reader.scroll.AtBottom()
		a.reader.replace(workspaceChangesDocument(a.session.Workspace, changes), true, follow)
	}
}

type workspaceChangeMonitor struct {
	workspace  string
	repository workspace.ChangeReader
	source     changefeed.Source
	apply      func([]workspace.Change) error
}

func (monitor workspaceChangeMonitor) run(ctx context.Context) {
	if monitor.source == nil || !monitor.source.Supports(changefeed.FilesChanged) {
		monitor.runWithoutWatch(ctx)
		return
	}
	subscription := changefeed.Subscription{
		Topics:  []changefeed.Topic{changefeed.FilesChanged},
		Watches: []changefeed.Watch{{ID: workspaceWatchID, Workspace: monitor.workspace}},
	}
	failures := 0
	for context.Cause(ctx) == nil {
		attemptContext, cancelAttempt := context.WithCancel(ctx)
		stream, err := monitor.source.Subscribe(attemptContext, subscription)
		if err != nil {
			cancelAttempt()
			failures++
			if reconnect.Wait(ctx, workspaceRetryDelay(failures)) != nil {
				return
			}
			continue
		}
		// The watch is registered before the cold read. Events that race the read
		// are buffered by the subscription and consumed below, closing the classic
		// read-then-subscribe gap without treating event payloads as state.
		if err := monitor.refresh(attemptContext); err != nil {
			cancelAttempt()
			failures++
			if reconnect.Wait(ctx, workspaceRetryDelay(failures)) != nil {
				return
			}
			continue
		}
		failures = 0
		lastSequence := uint64(0)
		for event, streamErr := range stream {
			if streamErr != nil {
				break
			}
			gap := lastSequence > 0 && event.Sequence != lastSequence+1
			lastSequence = event.Sequence
			if gap || monitor.invalidates(event) {
				if err := monitor.refresh(attemptContext); err != nil {
					break
				}
			}
		}
		cancelAttempt()
		failures++
		if reconnect.Wait(ctx, workspaceRetryDelay(failures)) != nil {
			return
		}
	}
}

func (monitor workspaceChangeMonitor) runWithoutWatch(ctx context.Context) {
	failures := 0
	for context.Cause(ctx) == nil {
		if err := monitor.refresh(ctx); err == nil {
			return
		}
		failures++
		if reconnect.Wait(ctx, workspaceRetryDelay(failures)) != nil {
			return
		}
	}
}

func (monitor workspaceChangeMonitor) refresh(ctx context.Context) error {
	changes, err := monitor.repository.Changes(ctx, monitor.workspace)
	if err != nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return monitor.apply(changes)
}

func (monitor workspaceChangeMonitor) invalidates(event changefeed.Event) bool {
	switch event.Type {
	case changefeed.EventType(changefeed.FilesChanged):
		return event.WatchID == workspaceWatchID && event.Workspace == monitor.workspace
	case changefeed.Resync:
		return containsTopic(event.Topics, changefeed.FilesChanged) || containsString(event.WatchIDs, workspaceWatchID)
	default:
		return false
	}
}

func workspaceRetryDelay(failures int) time.Duration {
	shift := min(max(failures-1, 0), 6)
	return min(100*time.Millisecond*time.Duration(1<<shift), 5*time.Second)
}

func containsTopic(values []changefeed.Topic, target changefeed.Topic) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
