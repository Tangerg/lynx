package terminal

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
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
			recovery:           runtimeRecoveryBackoff,
			subscriptionLimits: a.runtimeChangeSubscriptionLimits(),
			watchFiles:         a.runtimeSupports(runtimeprofile.FeatureFileWatch),
			resources:          a.observedRuntimeResources(),
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
		if err := monitor.run(ctx); err != nil && context.Cause(ctx) == nil {
			_ = post(ctx, dispatcher, func() {
				if !a.operations.Current(lease) || a.closed || a.session.Workspace.Path != workspacePath {
					return
				}
				a.message("runtime change observation stopped: " + err.Error())
			})
		}
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
	workspace          string
	repository         workspace.ChangeReader
	source             changefeed.Source
	recovery           retry.Backoff
	watchFiles         bool
	resources          runtimeResourceObservation
	applyFiles         func([]workspace.Change) error
	applyEvent         func(changefeed.Event) error
	applyResync        func([]changefeed.Topic) error
	subscriptionLimits changefeed.SubscriptionLimits
}

type runtimeResourceObservation struct {
	plan        bool
	goals       bool
	skills      bool
	mcp         bool
	schedules   bool
	knowledge   bool
	hooks       bool
	models      bool
	approvals   bool
	agentMemory bool
}

func (observation runtimeResourceObservation) hasWorkspaceAuthoredResources() bool {
	return observation.knowledge || observation.hooks
}

func (a *app) observedRuntimeResources() runtimeResourceObservation {
	return runtimeResourceObservation{
		plan:        a.runtimeSupports(runtimeprofile.FeaturePlan),
		goals:       a.goals != nil && a.runtimeSupports(runtimeprofile.FeatureGoals),
		skills:      a.skills != nil && a.runtimeSupports(runtimeprofile.FeatureSkills),
		mcp:         a.mcp != nil && a.runtimeSupports(runtimeprofile.FeatureMCP),
		schedules:   a.schedules != nil && a.runtimeSupports(runtimeprofile.FeatureSchedules),
		knowledge:   a.knowledge != nil && a.runtimeSupports(runtimeprofile.FeatureKnowledge),
		hooks:       a.hooks != nil,
		models:      true,
		approvals:   true,
		agentMemory: a.agentMemory != nil && a.runtimeSupports(runtimeprofile.FeatureAgentMemory),
	}
}

func (monitor runtimeChangeMonitor) observesWorkspace() bool {
	return monitor.watchFiles && (monitor.repository != nil || monitor.resources.hasWorkspaceAuthoredResources())
}

func (monitor runtimeChangeMonitor) run(ctx context.Context) error {
	topics := monitor.supportedTopics()
	if monitor.source == nil || len(topics) == 0 {
		return monitor.runWithoutWatch(ctx)
	}
	requested := changefeed.Subscription{Topics: topics}
	if monitor.observesWorkspace() && containsTopic(topics, changefeed.FilesChanged) {
		requested.Watches = []changefeed.Watch{{ID: workspaceWatchID, Workspace: monitor.workspace}}
	}
	subscriptions, err := monitor.subscriptionLimits.Partition(requested)
	if err != nil {
		return fmt.Errorf("plan runtime change subscriptions: %w", err)
	}
	if len(subscriptions) == 1 {
		return monitor.runSubscription(ctx, subscriptions[0], monitor.repository != nil)
	}
	return monitor.runSubscriptions(ctx, subscriptions)
}

func (a *app) runtimeChangeSubscriptionLimits() changefeed.SubscriptionLimits {
	if a.runtimeProfile == nil {
		return changefeed.SubscriptionLimits{}
	}
	limits := a.runtimeProfile.Limits.RuntimeSubscription
	return changefeed.SubscriptionLimits{MaxTopics: limits.MaxTopics, MaxWatches: limits.MaxWatches}
}

func (monitor runtimeChangeMonitor) runSubscriptions(ctx context.Context, subscriptions []changefeed.Subscription) error {
	groupContext, cancelGroup := context.WithCancelCause(ctx)
	defer cancelGroup(nil)

	fileOwner := 0
	for index, subscription := range subscriptions {
		if containsTopic(subscription.Topics, changefeed.FilesChanged) {
			fileOwner = index
			break
		}
	}
	results := make(chan error, len(subscriptions))
	for index, subscription := range subscriptions {
		ownsFileProjection := monitor.repository != nil && index == fileOwner
		go func(subscription changefeed.Subscription, ownsFileProjection bool) {
			results <- monitor.runSubscription(groupContext, subscription, ownsFileProjection)
		}(subscription, ownsFileProjection)
	}

	first := <-results
	cancelGroup(first)
	for range len(subscriptions) - 1 {
		<-results
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return first
}

func (monitor runtimeChangeMonitor) runSubscription(
	ctx context.Context,
	subscription changefeed.Subscription,
	ownsFileProjection bool,
) error {
	topics := subscription.Topics
	setupFailures, streamFailures := 0, 0
	for context.Cause(ctx) == nil {
		attemptContext, cancelAttempt := context.WithCancel(ctx)
		stream, err := monitor.source.Subscribe(attemptContext, subscription)
		if err != nil {
			cancelAttempt()
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			if !reconnect.Retryable(err) {
				return err
			}
			setupFailures++
			if retry.Wait(ctx, monitor.recoveryDelay(setupFailures)) != nil {
				return context.Cause(ctx)
			}
			continue
		}
		// The subscription is registered before every authoritative cold refresh.
		// Events that race those reads remain buffered in the stream and trigger a
		// later replacement read, closing read-then-subscribe gaps for every topic.
		// Query support and subscription support are independent capabilities.
		// Even when this runtime cannot watch files.changed, install the
		// authoritative file projection instead of leaving the header empty.
		if ownsFileProjection {
			if err := monitor.refreshFiles(attemptContext); err != nil {
				cancelAttempt()
				if cause := context.Cause(ctx); cause != nil {
					return cause
				}
				if !reconnect.Retryable(err) {
					return err
				}
				setupFailures++
				if retry.Wait(ctx, monitor.recoveryDelay(setupFailures)) != nil {
					return context.Cause(ctx)
				}
				continue
			}
		}
		if err := monitor.resync(topics); err != nil {
			cancelAttempt()
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			if !reconnect.Retryable(err) {
				return err
			}
			setupFailures++
			if retry.Wait(ctx, monitor.recoveryDelay(setupFailures)) != nil {
				return context.Cause(ctx)
			}
			continue
		}
		setupFailures = 0
		lastSequence := uint64(0)
		progressed := false
		var attemptErr error
		for event, streamErr := range stream {
			if streamErr != nil {
				attemptErr = streamErr
				break
			}
			gap := event.Sequence != lastSequence+1
			lastSequence = event.Sequence
			if gap {
				if ownsFileProjection && containsTopic(topics, changefeed.FilesChanged) {
					if err := monitor.refreshFiles(attemptContext); err != nil {
						attemptErr = err
						break
					}
				}
				if err := monitor.resync(topics); err != nil {
					attemptErr = err
					break
				}
				// The authoritative reads started after this frame was
				// observed, so they include both the missing changes and the
				// frame itself. Applying it again would only restart the same
				// projections and can starve convergence on a gappy stream.
				progressed = true
				continue
			}
			if ownsFileProjection && monitor.invalidatesFiles(event) {
				if err := monitor.refreshFiles(attemptContext); err != nil {
					attemptErr = err
					break
				}
			}
			if event.Type != changefeed.EventType(changefeed.FilesChanged) {
				if err := monitor.invalidate(event); err != nil {
					attemptErr = err
					break
				}
			}
			progressed = true
		}
		cancelAttempt()
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		if attemptErr == nil {
			attemptErr = fmt.Errorf("%w: runtime change stream ended", agent.ErrDisconnected)
		}
		if !reconnect.Retryable(attemptErr) {
			return attemptErr
		}
		if progressed {
			streamFailures = 0
		}
		streamFailures++
		if retry.Wait(ctx, monitor.recoveryDelay(streamFailures)) != nil {
			return context.Cause(ctx)
		}
	}
	return context.Cause(ctx)
}

func (monitor runtimeChangeMonitor) recoveryDelay(failure int) time.Duration {
	backoff := monitor.recovery
	if backoff.Base <= 0 && backoff.Maximum <= 0 {
		backoff = runtimeRecoveryBackoff
	}
	return backoff.Delay(failure)
}

func (monitor runtimeChangeMonitor) runWithoutWatch(ctx context.Context) error {
	if monitor.repository == nil {
		return nil
	}
	failures := 0
	for context.Cause(ctx) == nil {
		if err := monitor.refreshFiles(ctx); err == nil {
			return nil
		} else if !reconnect.Retryable(err) {
			return err
		}
		failures++
		if retry.Wait(ctx, monitor.recoveryDelay(failures)) != nil {
			return context.Cause(ctx)
		}
	}
	return context.Cause(ctx)
}

func (monitor runtimeChangeMonitor) supportedTopics() []changefeed.Topic {
	if monitor.source == nil {
		return nil
	}
	candidates := []changefeed.Topic{changefeed.SessionsChanged, changefeed.RunsChanged}
	if monitor.resources.plan {
		candidates = append(candidates, changefeed.StateChanged)
	}
	candidates = append(candidates, changefeed.InterruptsChanged)
	if monitor.resources.goals {
		candidates = append(candidates, changefeed.GoalsChanged)
	}
	if monitor.resources.skills {
		candidates = append(candidates, changefeed.SkillsChanged)
	}
	if monitor.resources.mcp {
		candidates = append(candidates, changefeed.MCPChanged)
	}
	if monitor.resources.schedules {
		candidates = append(candidates, changefeed.SchedulesChanged)
	}
	if monitor.resources.knowledge {
		candidates = append(candidates, changefeed.KnowledgeChanged)
	}
	if monitor.resources.hooks {
		candidates = append(candidates, changefeed.HooksChanged)
	}
	if monitor.resources.models {
		candidates = append(candidates, changefeed.ModelsChanged)
	}
	if monitor.resources.approvals {
		candidates = append(candidates, changefeed.ApprovalsChanged)
	}
	if monitor.resources.agentMemory {
		candidates = append(candidates, changefeed.AgentMemoryChanged)
	}
	if monitor.observesWorkspace() {
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
	if errors.Is(err, workspace.ErrVersionControlUnavailable) {
		changes = nil
	} else if err != nil {
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
		if event.WatchID != "" {
			return event.WatchID == workspaceWatchID &&
				(event.Workspace == "" || event.Workspace == monitor.workspace)
		}
		// Agent tool writes are broad file invalidations. They carry the
		// affected workspace but no client watch identity, and must refresh the
		// same authoritative projection as a watch-produced signal.
		return event.Workspace == "" || event.Workspace == monitor.workspace
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
