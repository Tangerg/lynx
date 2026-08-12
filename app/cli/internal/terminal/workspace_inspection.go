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
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
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
				if value.Workspace.ProjectRoot != "" && value.Workspace.ProjectRoot != value.Workspace.Path {
					label += "  · project " + value.Workspace.ProjectRoot
				}
				if value.LastActive != nil {
					label += "  · active " + value.LastActive.Format(time.RFC3339)
				}
				lines = append(lines, label)
			}
			return paragraphDocument("Runtime workspaces", fmt.Sprintf("%d known", len(values)), lines), nil
		}, workspaceReaderNone)
}

func (a *app) ShowWorkspaceChanges() {
	path := a.session.Workspace.Path
	a.runWorkspaceQuery("loading workspace changes",
		func(ctx context.Context) (readerDocument, error) {
			changes, err := a.workspaces.Changes(ctx, path)
			if err != nil {
				return readerDocument{}, err
			}
			return workspaceChangesDocument(path, changes), nil
		}, workspaceReaderChanges)
}

func (a *app) ShowWorkspaceDiff(argument string) error {
	selection, err := parseWorkspaceDiffSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.DiffRequest{
		Workspace: a.session.Workspace.Path, Path: selection.path,
		Mode: selection.mode, Format: selection.format, Limit: selection.limit,
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
			detail := string(request.Mode) + " · " + string(request.Format)
			if request.Path != "" {
				detail += " · " + request.Path
			}
			if diff.Truncated {
				detail += " · truncated"
			}
			return readerDocument{Title: "Workspace diff", Detail: detail, Sections: []ToolSection{{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: text}}}, nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) PreviewWorkspaceFile(argument string) error {
	selection, err := parseWorkspaceHeadSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.HeadRequest{Workspace: a.session.Workspace.Path, Path: selection.path, Lines: selection.lines}
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
			detail := fmt.Sprintf("%s · up to %d lines", head.Path, request.Lines)
			return codeDocument("File preview", detail, strings.Join(lines, "\n"), head.Path, true), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) SearchWorkspace(argument string) error {
	selection, err := parseWorkspaceSearchSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.SearchRequest{
		Workspace: a.session.Workspace.Path, Query: selection.query, Path: selection.path, Limit: selection.limit,
	}
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
			if request.Path != "" {
				detail += " · " + request.Path
			}
			detail += fmt.Sprintf(" · limit %d", request.Limit)
			return paragraphDocument("Workspace search · "+request.Query, detail, lines), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) BrowseWorkspace(argument string) error {
	selection, err := parseWorkspaceFilesSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.FilesRequest{
		Workspace: a.session.Workspace.Path, Path: selection.path, Glob: selection.glob,
		Recursive: selection.recursive, IncludeIgnored: selection.includeIgnored,
	}
	a.runWorkspaceQuery("browsing workspace",
		func(ctx context.Context) (readerDocument, error) {
			listing, err := a.workspaces.Files(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(listing.Entries))
			for _, entry := range listing.Entries {
				kind := string(entry.Type)
				if entry.Type == workspace.FileEntryDirectory {
					kind = "dir"
				}
				line := fmt.Sprintf("%-7s %s", kind, entry.Path)
				var metadata []string
				if entry.SizeBytes != nil {
					metadata = append(metadata, fmt.Sprintf("%d B", *entry.SizeBytes))
				}
				if entry.ModifiedAt != "" {
					metadata = append(metadata, entry.ModifiedAt)
				}
				if len(metadata) > 0 {
					line += "  · " + strings.Join(metadata, " · ")
				}
				lines = append(lines, line)
			}
			details := []string{fmt.Sprintf("%d entries", len(listing.Entries))}
			if request.Glob != "" {
				details = append(details, "glob "+request.Glob)
			}
			if request.Recursive {
				details = append(details, "recursive")
			}
			if request.IncludeIgnored {
				details = append(details, "including ignored")
			}
			title := "Workspace files"
			if request.Path != "" {
				title += " · " + request.Path
			}
			return codeDocument(title, strings.Join(details, " · "), strings.Join(lines, "\n"), "text", false), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) ReadWorkspaceFile(argument string) error {
	selection, err := parseWorkspaceReadSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.ReadRequest{
		Workspace: a.session.Workspace.Path, Path: selection.path,
		StartLine: selection.startLine, EndLine: selection.endLine, MaxBytes: selection.maxBytes,
	}
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
	return nil
}

func (a *app) runWorkspaceQuery(status string, query func(context.Context) (readerDocument, error), mode workspaceReaderMode) {
	a.status.note(status)
	runOperation(a, readerDocumentOperation, true, query, func(document readerDocument, err error) {
		if err != nil {
			a.message("workspace: " + err.Error())
			return
		}
		a.workspaceReader = mode
		a.setRuntimeReader(runtimeReaderNone)
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

func (a *app) followRuntimeChanges() {
	a.operations.Cancel(runtimeChangesOperation)
	workspacePath := a.session.Workspace.Path
	var repository workspace.ChangeReader
	if a.runtimeSupports(runtimeprofile.FeatureGit) {
		repository = a.workspaces
	}
	if repository == nil && a.changes == nil {
		return
	}
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(runtimeChangesOperation, true, func(ctx context.Context, lease operationLease) {
		monitor := runtimeChangeMonitor{
			workspace: workspacePath, repository: repository, source: a.changes,
			watchFiles:   a.runtimeSupports(runtimeprofile.FeatureFileWatch),
			includeGoals: a.goals != nil, includeSkills: a.skills != nil, includeMCP: a.mcp != nil,
			includeSchedules: a.schedules != nil,
			applyFiles: func(changes []workspace.Change) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.Workspace.Path != workspacePath {
						return
					}
					a.applyWorkspaceChanges(changes)
				})
			},
			applyEvent: func(event changefeed.Event) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.Workspace.Path != workspacePath {
						return
					}
					a.applyRuntimeInvalidation(event)
				})
			},
			applyResync: func(topics []changefeed.Topic) error {
				return post(ctx, dispatcher, func() {
					if !a.operations.Current(lease) || a.closed || a.session.Workspace.Path != workspacePath {
						return
					}
					a.applyRuntimeResync(topics)
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
		a.reader.replace(workspaceChangesDocument(a.session.Workspace.Path, changes), true, follow)
	}
}

type runtimeChangeMonitor struct {
	workspace        string
	repository       workspace.ChangeReader
	source           changefeed.Source
	watchFiles       bool
	includeGoals     bool
	includeSkills    bool
	includeMCP       bool
	includeSchedules bool
	applyFiles       func([]workspace.Change) error
	applyEvent       func(changefeed.Event) error
	applyResync      func([]changefeed.Topic) error
}

func (monitor runtimeChangeMonitor) run(ctx context.Context) {
	topics := monitor.supportedTopics()
	if monitor.source == nil || len(topics) == 0 {
		monitor.runWithoutWatch(ctx)
		return
	}
	subscription := changefeed.Subscription{Topics: topics}
	if monitor.repository != nil && monitor.watchFiles && containsTopic(topics, changefeed.FilesChanged) {
		subscription.Watches = []changefeed.Watch{{ID: workspaceWatchID, Workspace: monitor.workspace}}
	}
	failures := 0
	for context.Cause(ctx) == nil {
		attemptContext, cancelAttempt := context.WithCancel(ctx)
		stream, err := monitor.source.Subscribe(attemptContext, subscription)
		if err != nil {
			cancelAttempt()
			failures++
			if reconnect.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)) != nil {
				return
			}
			continue
		}
		// The subscription is registered before every authoritative cold refresh.
		// Events that race those reads remain buffered in the stream and trigger a
		// later replacement read, closing read-then-subscribe gaps for every topic.
		// Query support and subscription support are independent capabilities.
		// Even when this runtime cannot watch files.changed, install the
		// authoritative file projection instead of leaving the header empty.
		if monitor.repository != nil {
			if err := monitor.refreshFiles(attemptContext); err != nil {
				cancelAttempt()
				failures++
				if reconnect.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)) != nil {
					return
				}
				continue
			}
		}
		if err := monitor.resync(topics); err != nil {
			cancelAttempt()
			failures++
			if reconnect.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)) != nil {
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
			gap := event.Sequence != lastSequence+1
			lastSequence = event.Sequence
			if gap {
				if containsTopic(topics, changefeed.FilesChanged) {
					if err := monitor.refreshFiles(attemptContext); err != nil {
						break
					}
				}
				if err := monitor.resync(topics); err != nil {
					break
				}
			}
			if monitor.invalidatesFiles(event) {
				if err := monitor.refreshFiles(attemptContext); err != nil {
					break
				}
			}
			if event.Type != changefeed.EventType(changefeed.FilesChanged) {
				if err := monitor.invalidate(event); err != nil {
					break
				}
			}
		}
		cancelAttempt()
		failures++
		if reconnect.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)) != nil {
			return
		}
	}
}

func (monitor runtimeChangeMonitor) runWithoutWatch(ctx context.Context) {
	if monitor.repository == nil {
		return
	}
	failures := 0
	for context.Cause(ctx) == nil {
		if err := monitor.refreshFiles(ctx); err == nil {
			return
		}
		failures++
		if reconnect.Wait(ctx, runtimeRecoveryBackoff.Delay(failures)) != nil {
			return
		}
	}
}

func (monitor runtimeChangeMonitor) supportedTopics() []changefeed.Topic {
	if monitor.source == nil {
		return nil
	}
	candidates := []changefeed.Topic{
		changefeed.SessionsChanged,
		changefeed.RunsChanged,
		changefeed.StateChanged,
		changefeed.InterruptsChanged,
	}
	if monitor.includeGoals {
		candidates = append(candidates, changefeed.GoalsChanged)
	}
	if monitor.includeSkills {
		candidates = append(candidates, changefeed.SkillsChanged)
	}
	if monitor.includeMCP {
		candidates = append(candidates, changefeed.MCPChanged)
	}
	if monitor.includeSchedules {
		candidates = append(candidates, changefeed.SchedulesChanged)
	}
	if monitor.repository != nil && monitor.watchFiles {
		candidates = append([]changefeed.Topic{changefeed.FilesChanged}, candidates...)
	}
	topics := make([]changefeed.Topic, 0, len(candidates))
	for _, topic := range candidates {
		if monitor.source.Supports(topic) {
			topics = append(topics, topic)
		}
	}
	return topics
}

func (monitor runtimeChangeMonitor) refreshFiles(ctx context.Context) error {
	if monitor.repository == nil || monitor.applyFiles == nil {
		return nil
	}
	changes, err := monitor.repository.Changes(ctx, monitor.workspace)
	if err != nil {
		return err
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return monitor.applyFiles(changes)
}

func (monitor runtimeChangeMonitor) invalidate(event changefeed.Event) error {
	if monitor.applyEvent == nil {
		return nil
	}
	return monitor.applyEvent(event)
}

func (monitor runtimeChangeMonitor) resync(topics []changefeed.Topic) error {
	if monitor.applyResync == nil {
		return nil
	}
	return monitor.applyResync(topics)
}

func (monitor runtimeChangeMonitor) invalidatesFiles(event changefeed.Event) bool {
	switch event.Type {
	case changefeed.EventType(changefeed.FilesChanged):
		return event.WatchID == workspaceWatchID && event.Workspace == monitor.workspace
	case changefeed.Resync:
		return containsTopic(event.Topics, changefeed.FilesChanged) || containsString(event.WatchIDs, workspaceWatchID)
	default:
		return false
	}
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
