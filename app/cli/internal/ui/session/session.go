// Package session is the interactive terminal adapter for one conversation.
// It owns oolong state and translates user intent into the CLI's runtime port;
// neither the domain model nor a runtime adapter imports this package.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

const (
	sendPrompt      keymap.Action = settings.ActionSend
	cancelRun       keymap.Action = settings.ActionCancelRun
	quitApp         keymap.Action = settings.ActionQuit
	commandPalette  keymap.Action = settings.ActionCommandPalette
	showSessions    keymap.Action = settings.ActionSessions
	searchHistory   keymap.Action = settings.ActionSearch
	cycleMode       keymap.Action = settings.ActionCycleMode
	toggleDetails   keymap.Action = settings.ActionToggleDetails
	historyPrevious keymap.Action = settings.ActionHistoryPrevious
	historyNext     keymap.Action = settings.ActionHistoryNext
	nextMatch       keymap.Action = settings.ActionNextMatch
	previousMatch   keymap.Action = settings.ActionPreviousMatch
)

// runtime is the product capability this adapter consumes. A future real
// adapter and today's mock satisfy the same interface without either learning
// about oolong.
type runtime interface {
	client.SessionCatalog
	client.SessionReader
	client.SessionWriter
	client.Runs
	client.Models
	client.Approvals
}

// Config describes one interactive session.
type Config struct {
	Runtime       runtime
	Session       string
	Workspace     string
	Prompt        string
	Plugins       []extensions.Plugin
	PluginSources []extensions.Source
	Host          program.Host
	Settings      settings.Settings
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) (runErr error) {
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}
	cfg = prepared.config

	registry := new(extensions.Registry)
	kernel, err := extensions.NewKernel(registry)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, kernel.Close()) }()
	sources := make([]extensions.Source, 0, 1+len(cfg.PluginSources))
	sources = append(sources, extensions.StaticSource{
		Name: "terminal", Plugins: append([]extensions.Plugin{builtinPlugin()}, cfg.Plugins...),
	})
	sources = append(sources, cfg.PluginSources...)
	discovered, err := extensions.Discover(ctx, sources...)
	if err != nil {
		return err
	}
	results, err := kernel.Activate(discovered.Plugins)
	if err != nil {
		return err
	}
	if err := requirePlugin(results, "terminal.core"); err != nil {
		return err
	}

	var active *app
	err = program.Run(ctx, program.Config{
		Inline: func(loop *program.InlineRuntime) program.Component {
			active = newApp(ctx, loop, cfg.Runtime, prepared.opened, registry, kernel, discovered.Issues, prepared.attachments, cfg.Prompt, cfg.Settings, prepared.keys)
			return headless.NewRoot(active)
		},
		Terminal: term.Options{Probe: true, Mouse: cfg.Settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	})
	if active != nil {
		active.Close(ctx)
	}
	return err
}

type preparedSession struct {
	config      Config
	opened      client.SessionSnapshot
	attachments *attachment.Resolver
	keys        *keymap.Map
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	if cfg.Runtime == nil {
		return preparedSession{}, errors.New("session: a runtime is required")
	}
	if cfg.Settings.Keys == nil {
		cfg.Settings = settings.Default()
	}
	if err := cfg.Settings.Validate(); err != nil {
		return preparedSession{}, fmt.Errorf("session settings: %w", err)
	}
	keys, err := configuredKeys(cfg.Settings)
	if err != nil {
		return preparedSession{}, err
	}
	opened, err := open(ctx, cfg.Runtime, cfg.Session, cfg.Workspace)
	if err != nil {
		return preparedSession{}, err
	}
	attachments, err := attachment.New(opened.Session.Workspace)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session attachments: %w", err)
	}
	return preparedSession{config: cfg, opened: opened, attachments: attachments, keys: keys}, nil
}

func requirePlugin(results []extensions.Result, id string) error {
	for _, result := range results {
		if result.PluginID != id {
			continue
		}
		if result.Phase == extensions.PluginLoaded {
			return nil
		}
		if result.Err != nil {
			return fmt.Errorf("session: required plugin %q is %s: %w", id, result.Phase, result.Err)
		}
		return fmt.Errorf("session: required plugin %q is %s", id, result.Phase)
	}
	return fmt.Errorf("session: required plugin %q was not discovered", id)
}

func open(ctx context.Context, rt interface {
	client.SessionReader
	client.SessionWriter
}, id, workspace string) (client.SessionSnapshot, error) {
	if id == "" {
		created, err := rt.CreateSession(ctx, client.NewSession{Workspace: workspace})
		if err != nil {
			return client.SessionSnapshot{}, err
		}
		snapshot := client.SessionSnapshot{Session: created}
		if err := snapshot.Validate(); err != nil {
			return client.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
		}
		return snapshot, nil
	}
	snapshot, err := rt.GetSession(ctx, id)
	if err != nil {
		return client.SessionSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return client.SessionSnapshot{}, fmt.Errorf("open session: %w", err)
	}
	return snapshot, nil
}

type app struct {
	ctx          context.Context
	loop         *program.InlineRuntime
	backend      runtime
	session      client.Session
	registry     *extensions.Registry
	plugins      *extensions.Kernel
	pluginIssues []extensions.SourceIssue
	state        *client.Conversation
	operations   *operationOwner

	transcript *conversationView
	workflow   workflowView
	status     statusView
	settings   settings.Settings
	options    client.RunOptions
	composer   kit.Composer
	commands   headless.Commands
	completion headless.Completion
	body       *headless.Container
	stack      headless.Stack

	review             *client.Approval
	reviewChoice       string
	reviewForm         *headless.Form
	reviewPane         reviewPane
	reviewDialog       *kit.Dialog
	sessionPicker      *picker[client.Session]
	sessionDialog      *kit.Dialog
	modelPicker        *picker[client.Model]
	modelDialog        *kit.Dialog
	permissionPicker   *picker[client.PermissionMode]
	permissionDialog   *kit.Dialog
	question           *client.Question
	questionDialog     *kit.Dialog
	questionText       map[string]*string
	questionMulti      map[string]*[]string
	questionBool       map[string]*bool
	commandPicker      *picker[headless.Command]
	commandDialog      *kit.Dialog
	searchDialog       *kit.Dialog
	searchQuery        string
	attachments        *attachment.Resolver
	attachmentElements map[uint64]client.Attachment
	history            promptHistory
	commandSeq         uint64
	commandOperations  map[uint64]commandOperation

	streamSeq     uint64
	startRequest  string
	pendingCancel *client.CancelRun
	following     bool
	stopClock     func()
	started       time.Time
	closed        bool
	syntax        highlight.Style
}

func newApp(ctx context.Context, loop *program.InlineRuntime, backend runtime, opened client.SessionSnapshot, registry *extensions.Registry, plugins *extensions.Kernel, pluginIssues []extensions.SourceIssue, attachments *attachment.Resolver, prompt string, configured settings.Settings, keys *keymap.Map) *app {
	ground := loop.Environment().Ground()
	theme := kit.Suited(ground)
	glyphs := kit.GlyphsFor(os.LookupEnv)
	syntax := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		syntax = "github"
	}
	a := &app{
		ctx: ctx, loop: loop, backend: backend, session: opened.Session, registry: registry,
		plugins: plugins, pluginIssues: pluginIssues,
		state:              client.NewConversation(),
		operations:         newOperationOwner(ctx),
		transcript:         newConversationView(theme, glyphs, loop.Environment().Wheel(), syntax, configured.UI.TranscriptRetain, configured.UI.ToolDetails),
		workflow:           newWorkflowView(theme, glyphs),
		status:             statusView{theme: theme, glyphs: glyphs, doing: "ready", options: configured.RunOptions()},
		settings:           configured.Clone(),
		options:            configured.RunOptions(),
		syntax:             syntax,
		attachments:        attachments,
		attachmentElements: make(map[uint64]client.Attachment),
		commandOperations:  make(map[uint64]commandOperation),
	}
	a.composer = kit.Composer{
		Theme: theme, Prompt: glyphs.Marker + " ",
		Hints: []keymap.Action{sendPrompt, cancelRun, showSessions, cycleMode, quitApp}, MaxRows: 6,
	}
	a.composer.Editor().Placeholder = "Ask lyra to inspect, explain, or change something"
	a.composer.Editor().Keys = keys
	a.composer.Editor().Clipboard = loop.Clipboard()
	if prompt != "" {
		a.composer.Editor().SetText(prompt)
	}

	completionKeys := headless.DefaultCompletionKeys()
	completionKeys.Bind(headless.Accept, input.Chord{Code: input.Enter})
	a.completion = headless.Completion{
		Look: theme.Look(glyphs), Keys: completionKeys,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			if token.Trigger.Prefix == "@" {
				a.composer.Editor().Replace(max(token.Start-1, 0), token.End, "")
				if err := a.addAttachment(candidate.Text); err != nil {
					a.message(err.Error())
				}
				return
			}
			a.composer.Editor().Replace(token.Start, token.End, candidate.Text)
		},
	}
	a.registerCommands()

	a.body = headless.Rows(
		headless.Item{Key: "transcript", Size: layout.Flex(1), Of: a.transcript},
		headless.Item{Key: "plan", Size: layout.Measured(0, 8), Of: headless.Static{Of: &a.workflow}},
		headless.Item{Key: "status", Size: layout.Fixed(1), Of: headless.Static{Of: &a.status}},
		headless.Item{Key: "composer", Size: layout.Measured(1, 8), Of: &a.composer},
	)
	a.body.Focus(true)
	a.stack.SetBase(a.body)
	a.buildReview(theme, glyphs)
	a.buildSessionPicker(theme, glyphs)
	a.buildRuntimePickers(theme, glyphs)
	a.buildCommandPalette(theme, glyphs)
	a.buildSearchDialog(theme, glyphs)
	a.listenForSearch()
	loop.Session().SetTitle("lyra — " + displayTitle(opened.Session))
	a.restore(opened)
	return a
}

func (a *app) buildSessionPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.sessionPicker = newPicker(theme, glyphs, "search sessions",
		displayTitle,
		func(session client.Session) string { return agoShort(session.UpdatedAt) + " · " + session.Workspace },
		func(session client.Session) {
			a.sessionDialog.Dismiss()
			a.switchSession(session.ID)
		},
	)
	a.sessionDialog = kit.NewDialog(&a.stack, theme, glyphs, "Sessions", a.sessionPicker)
	a.sessionDialog.Panel().Where = layout.Placement{Width: 88, Height: 18, Margin: 1}
	a.sessionPicker.cancel = a.sessionDialog.Dismiss
}

func (a *app) restore(snapshot client.SessionSnapshot) {
	if err := a.state.RestoreSnapshot(snapshot); err != nil {
		a.fail(err)
		return
	}
	if err := presentSnapshot(a.transcript, snapshot, a.registry); err != nil {
		a.fail(err)
		return
	}
	a.restoreActivity(snapshot)
}

func presentSnapshot(view *conversationView, snapshot client.SessionSnapshot, registry *extensions.Registry) error {
	for _, envelope := range snapshot.Events {
		if err := view.Apply(envelope.Event, registry); err != nil {
			return fmt.Errorf("restore transcript at cursor %d: %w", envelope.Cursor, err)
		}
	}
	return nil
}

func (a *app) restoreActivity(snapshot client.SessionSnapshot) {
	a.workflow.Set(a.state.Plan())
	switch a.state.Phase() {
	case client.Waiting:
		a.openInteraction(a.state.Interaction())
		a.status.note("waiting for your answer")
	case client.Running:
		if snapshot.Active == nil {
			a.fail(errors.New("session snapshot has a running conversation without an active run"))
			return
		}
		a.status.active("reconnecting")
		a.follow(func(context.Context) (subscription, error) {
			return subscription{runID: snapshot.Active.ID, after: snapshot.Cursor}, nil
		})
	case client.Idle:
		if a.state.Outcome().Status != "" {
			a.status.settled(a.state.Outcome(), a.state.Usage())
		}
	default:
		a.fail(errors.New("session snapshot has an unknown conversation phase"))
	}
}

func displayTitle(session client.Session) string {
	if strings.TrimSpace(session.Title) == "" {
		return "untitled"
	}
	return session.Title
}

func (a *app) Draw(frame headless.Frame) {
	a.stack.Draw(frame)
	if a.stack.Empty() && a.completion.Open() {
		a.drawCompletion(frame)
	}
}

func (a *app) Handle(event input.Event) bool {
	action := a.action(event)
	if a.handleGlobalAction(action) {
		return true
	}
	if !a.stack.Empty() {
		return a.stack.Handle(event)
	}
	if a.handleSessionAction(action) {
		return true
	}
	if a.completion.Handle(event) {
		return true
	}
	if a.handleHistoryAction(action) {
		return true
	}
	if isSubmitEvent(event) {
		a.submit()
		return true
	}
	handled := a.stack.Handle(event)
	if handled {
		a.refreshCompletion()
	}
	return handled
}

func (a *app) action(event input.Event) keymap.Action {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return ""
	}
	action, _ := a.composer.Editor().Keys.Action(key.Chord())
	return action
}

func (a *app) handleGlobalAction(action keymap.Action) bool {
	switch action {
	case quitApp:
		a.loop.Quit()
		return true
	case cancelRun:
		a.cancel()
		return true
	default:
		return false
	}
}

func (a *app) handleSessionAction(action keymap.Action) bool {
	switch action {
	case commandPalette:
		a.showCommandPalette()
		return true
	case showSessions:
		a.ShowSessions()
		return true
	case cycleMode:
		a.CycleMode()
		return true
	case searchHistory:
		a.showSearchDialog()
		return true
	case toggleDetails:
		a.ToggleToolDetails()
		return true
	case nextMatch:
		a.NextMatch()
		return true
	case previousMatch:
		a.PreviousMatch()
		return true
	default:
		return false
	}
}

func (a *app) handleHistoryAction(action keymap.Action) bool {
	switch action {
	case historyPrevious:
		if !a.recallPrevious() {
			a.composer.Editor().MoveUp()
		}
		return true
	case historyNext:
		if !a.recallNext() {
			a.composer.Editor().MoveDown()
		}
		return true
	default:
		return false
	}
}

func isSubmitEvent(event input.Event) bool {
	key, ok := event.(input.Key)
	return ok && key.Down() && key.Code == input.Enter && key.Mods == 0
}

func (a *app) Close(ctx context.Context) {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	target, cancelRuntime := a.activeCancellation()
	if !cancelRuntime && a.pendingCancel != nil {
		target, cancelRuntime = *a.pendingCancel, true
	}
	a.dropStream()
	a.operations.Cancel(completionOperation)
	a.cancelPluginCommands()
	a.operations.Close()
	if cancelRuntime {
		a.cancelRuntimeNow(ctx, target)
	}
	a.transcript.Close()
	if a.stopClock != nil {
		a.stopClock()
		a.stopClock = nil
	}
}

func (a *app) submit() {
	message, err := a.composerMessage()
	if err != nil {
		a.message(err.Error())
		return
	}
	if message.Text == "" && len(message.Attachments) == 0 {
		return
	}
	if name, arg, command := headless.Parse(message.Text); command {
		// A command acts on the staged composer context. Clear its command text but
		// put attachment elements back so /attachments and /detach can inspect or
		// mutate them without accidentally sending a user turn.
		a.restoreComposer(client.Message{Attachments: message.Attachments})
		a.operations.Cancel(completionOperation)
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run first")
		return
	}
	if a.pendingCancel != nil {
		a.message("wait for runtime cancellation to finish")
		return
	}
	if a.operations.Active(sessionChangeOperation) {
		a.message("wait for the current session change to finish")
		return
	}
	a.operations.Cancel(pickerCatalogOperation)
	a.sessionDialog.Dismiss()
	a.modelDialog.Dismiss()
	a.history.Add(message)
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	a.start(message)
}
