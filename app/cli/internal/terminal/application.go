package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

const (
	sendPrompt       keymap.Action = settings.ActionSend
	insertNewline    keymap.Action = settings.ActionNewline
	cancelRun        keymap.Action = settings.ActionCancelRun
	quitApp          keymap.Action = settings.ActionQuit
	commandPalette   keymap.Action = settings.ActionCommandPalette
	showShortcuts    keymap.Action = settings.ActionShortcuts
	showSessions     keymap.Action = settings.ActionSessions
	searchTranscript keymap.Action = settings.ActionSearch
	manageQueue      keymap.Action = settings.ActionManageQueue
	chooseModel      keymap.Action = settings.ActionChooseModel
	toggleDetails    keymap.Action = settings.ActionToggleDetails
	historyPrevious  keymap.Action = settings.ActionHistoryPrevious
	historyNext      keymap.Action = settings.ActionHistoryNext
	nextMatch        keymap.Action = settings.ActionNextMatch
	previousMatch    keymap.Action = settings.ActionPreviousMatch
	scrollPageUp     keymap.Action = settings.ActionScrollPageUp
	scrollPageDown   keymap.Action = settings.ActionScrollPageDown
	scrollTop        keymap.Action = settings.ActionScrollTop
	scrollBottom     keymap.Action = settings.ActionScrollBottom
	queueFollowUp    keymap.Action = "queue-follow-up"
	queueOrSendNext  keymap.Action = "queue-or-send-next"
)

type app struct {
	ctx          context.Context
	loop         *program.InlineRuntime
	runtime      agent.Runtime
	session      agent.Session
	registry     *extensions.Registry
	pluginHost   *extensions.Host
	pluginIssues []extensions.SourceIssue
	conversation *agent.Conversation
	operations   *operationOwner

	transcript  *transcriptView
	header      *sessionHeader
	activity    *activityView
	queueView   *queueView
	queueDrawer *queueDrawer
	status      *statusView
	settings    settings.Config
	options     agent.RunOptions
	composer    kit.Composer
	prompt      *promptView
	commands    headless.Commands
	completion  headless.Completion
	shell       *shellView
	stack       headless.Stack
	queue       *promptqueue.Queue

	approval           *agent.Approval
	approvalChoice     string
	approvalForm       *headless.Form
	approvalPane       approvalPane
	approvalDialog     *kit.Dialog
	sessionPicker      *picker[agent.Session]
	sessionDialog      *kit.Dialog
	modelPicker        *picker[agent.Model]
	modelDialog        *kit.Dialog
	approvalModePicker *picker[agent.ApprovalMode]
	approvalModeDialog *kit.Dialog
	question           *agent.Question
	questionDialog     *kit.Dialog
	questionText       map[int]*string
	questionMulti      map[int]*[]string
	interactionQueue   []agent.Interaction
	interactionAnswers []agent.InterruptAnswer
	commandPicker      *picker[headless.Command]
	commandDialog      *kit.Dialog
	shortcutDialog     *kit.Dialog
	shortcutViewport   *headless.Viewport
	searchDialog       *kit.Dialog
	queueDialog        *headless.Dialog
	searchQuery        string
	attachments        *attachment.Resolver
	attachmentElements map[uint64]agent.Attachment
	history            promptHistory
	commandSeq         uint64
	commandOperations  map[uint64]commandOperation
	confirmation       pressConfirmation
	applicationKeys    *keymap.Map
	globalKeys         *keymap.Map
	applicationMatcher keymap.Matcher
	globalMatcher      keymap.Matcher

	streamSeq             uint64
	dispatchingQueueEntry uint64
	pendingCancel         *agent.CancelRun
	following             bool
	stopClock             func()
	executionClock        activeDurationClock
	closed                bool
	syntax                highlight.Renderer
}

type appConfig struct {
	Context       context.Context
	Runtime       agent.Runtime
	Snapshot      agent.SessionSnapshot
	Registry      *extensions.Registry
	PluginHost    *extensions.Host
	PluginIssues  []extensions.SourceIssue
	Attachments   *attachment.Resolver
	InitialPrompt string
	Settings      settings.Config
	KeyBindings   keyBindings
	Queue         *promptqueue.Queue
}

func newApp(loop *program.InlineRuntime, cfg appConfig) *app {
	cfg.KeyBindings.setResolver(loop.After)
	editorKeys := cfg.KeyBindings.editor
	ground := loop.Environment().Ground()
	theme := kit.Suited(ground)
	glyphs := kit.GlyphsFor(loop.Environment().Locale())
	syntaxStyle := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		syntaxStyle = "github"
	}
	syntax := highlight.New(syntaxStyle)
	a := &app{
		ctx: cfg.Context, loop: loop, runtime: cfg.Runtime, session: cfg.Snapshot.Session, registry: cfg.Registry,
		pluginHost: cfg.PluginHost, pluginIssues: cfg.PluginIssues,
		conversation:       agent.NewConversation(),
		operations:         newOperationOwner(cfg.Context),
		transcript:         newTranscriptView(theme, glyphs, loop.Environment().Wheel(), syntax, cfg.Settings.UI.TranscriptRetain, cfg.Settings.UI.ToolDetails, loop.Clipboard()),
		header:             newSessionHeader(theme, glyphs, cfg.Snapshot.Session),
		activity:           newActivityView(theme, glyphs),
		queueView:          newQueueView(theme, glyphs),
		status:             newStatusView(theme, glyphs, cfg.Settings.RunOptions()),
		queue:              cfg.Queue,
		settings:           cfg.Settings.Clone(),
		options:            cfg.Settings.RunOptions(),
		syntax:             syntax,
		attachments:        cfg.Attachments,
		attachmentElements: make(map[uint64]agent.Attachment),
		commandOperations:  make(map[uint64]commandOperation),
		applicationKeys:    cfg.KeyBindings.application,
		globalKeys:         cfg.KeyBindings.global,
	}
	a.composer = kit.Composer{
		Theme: theme, Prompt: glyphs.Marker + " ",
		MaxRows: 6,
	}
	a.composer.Editor().Placeholder = "Ask lyra to inspect, explain, or change something"
	a.composer.Editor().Keys = editorKeys
	a.composer.Editor().Clipboard = loop.Clipboard()
	if cfg.InitialPrompt != "" {
		a.composer.Editor().SetText(cfg.InitialPrompt)
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

	a.prompt = newPromptView(theme, glyphs, editorKeys, &a.composer, a.options)
	a.shell = newShellView(a.header, a.transcript, a.activity, a.queueView, a.status, a.prompt)
	a.wireTranscript(a.transcript)
	a.shell.Focus(true)
	a.stack.SetBase(a.shell)
	a.buildQueueDrawer(theme, glyphs, editorKeys)
	a.buildApprovalDialog(theme, glyphs)
	a.buildSessionPicker(theme, glyphs)
	a.buildRuntimePickers(theme, glyphs)
	a.buildCommandPalette(theme, glyphs)
	a.buildShortcutDialog(theme, glyphs, editorKeys)
	a.buildSearchDialog(theme, glyphs)
	a.listenForSearch()
	loop.Session().SetTitle("lyra — " + displayTitle(cfg.Snapshot.Session))
	a.restore(cfg.Snapshot)
	return a
}

func (a *app) wireTranscript(transcript *transcriptView) {
	a.prompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(a.prompt.SetTranscriptFocused)
	transcript.OnSelection(a.prompt.SetTranscriptSelection)
	transcript.OnCopy(func(string) {
		if !a.conversation.Busy() && !a.following {
			a.status.note("copied selected transcript text")
		}
	})
}

func (a *app) buildSessionPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.sessionPicker = newPicker(theme, glyphs, "search sessions",
		displayTitle,
		func(session agent.Session) string {
			return compactRelativeAge(session.UpdatedAt) + " · " + session.Workspace
		},
		func(session agent.Session) {
			a.sessionDialog.Dismiss()
			a.switchSession(session.ID)
		},
	)
	a.sessionDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Sessions", Body: a.sessionPicker,
		Where: layout.Placement{Width: 88, Height: 18, Margin: 1},
	})
	a.sessionPicker.cancel = a.sessionDialog.Dismiss
}

func (a *app) restore(snapshot agent.SessionSnapshot) {
	if err := a.conversation.RestoreSnapshot(snapshot); err != nil {
		a.fail(err)
		return
	}
	if err := presentSnapshot(a.transcript, snapshot, a.registry); err != nil {
		a.fail(err)
		return
	}
	a.restoreActivity(snapshot)
}

func presentSnapshot(view *transcriptView, snapshot agent.SessionSnapshot, registry *extensions.Registry) error {
	for _, block := range snapshot.Transcript {
		var event agent.Event = agent.BlockCompleted{Block: block}
		if block.Status == agent.BlockStatusRunning {
			event = agent.BlockStarted{Block: block}
		}
		if err := view.Apply(event, registry); err != nil {
			return fmt.Errorf("restore transcript block %s: %w", block.ID, err)
		}
	}
	return nil
}

// reconcileRunSnapshot atomically replaces the in-memory projection after a
// segment can no longer be replayed. It deliberately keeps the current stream
// operation alive: runrecovery already attached the replacement stream before
// taking this snapshot, so canceling that operation here would reopen a gap.
func (a *app) reconcileRunSnapshot(snapshot agent.SessionSnapshot, stream agent.SegmentStream) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("reconcile run snapshot: %w", err)
	}
	if snapshot.Session.ID != a.session.ID {
		return fmt.Errorf("reconcile run snapshot: session %s does not match %s", snapshot.Session.ID, a.session.ID)
	}
	next := agent.NewConversation()
	var restoreErr error
	if active, ok := snapshot.ActiveRun(); ok && active.Status == agent.RunStatusRunning {
		restoreErr = next.RestoreAttachedSnapshot(snapshot, stream)
	} else {
		restoreErr = next.RestoreSnapshot(snapshot)
	}
	if restoreErr != nil {
		return fmt.Errorf("reconcile run snapshot: %w", restoreErr)
	}
	nextTranscript := newTranscriptView(
		a.transcript.theme, a.transcript.glyphs, a.transcript.wheel, a.syntax,
		a.settings.UI.TranscriptRetain, a.transcript.details, a.transcript.clipboard,
	)
	if err := presentSnapshot(nextTranscript, snapshot, a.registry); err != nil {
		nextTranscript.Close()
		return err
	}

	previousTranscript := a.transcript
	a.session = snapshot.Session
	a.conversation = next
	a.transcript = nextTranscript
	a.wireTranscript(nextTranscript)
	a.shell.SetTranscript(nextTranscript)
	a.header.SetSession(snapshot.Session)
	a.header.SetUsage(next.Usage())
	a.activity.Set(next.Plan())
	a.prompt.SetBusy(next.Busy())
	previousTranscript.Close()
	a.listenForSearch()
	a.loop.Session().SetTitle("lyra — " + displayTitle(snapshot.Session))

	switch next.Phase() {
	case agent.ConversationRunning:
		a.following = true
		a.executionClock.start(next.Usage().Duration, time.Now())
		a.status.active("reconnected")
	case agent.ConversationWaiting:
		a.following = false
		a.openInteractions(next.Interactions())
		a.status.note("waiting for your answers")
	case agent.ConversationIdle:
		a.following = false
		if next.Outcome().Status != "" {
			a.status.settled(next.Outcome(), next.Usage())
		}
		if a.drainQueue() {
			return nil
		}
		if a.settings.UI.Notifications && next.Outcome().Status != "" {
			a.loop.Session().Notify(outcomeNotification(next.Outcome()))
		}
	default:
		return errors.New("reconcile run snapshot: unknown conversation phase")
	}
	a.syncAnimation()
	return nil
}

func (a *app) restoreActivity(snapshot agent.SessionSnapshot) {
	a.activity.Set(a.conversation.Plan())
	a.header.SetUsage(a.conversation.Usage())
	a.prompt.SetBusy(a.conversation.Busy())
	switch a.conversation.Phase() {
	case agent.ConversationWaiting:
		a.openInteractions(a.conversation.Interactions())
		a.status.note("waiting for your answers")
	case agent.ConversationRunning:
		if _, ok := snapshot.ActiveRun(); !ok {
			a.fail(errors.New("session snapshot has a running conversation without an active run"))
			return
		}
		a.executionClock.start(a.conversation.Usage().Duration, time.Now())
		a.status.active("reconnecting")
		a.followRecoveredSession()
	case agent.ConversationIdle:
		if a.conversation.Outcome().Status != "" {
			a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
		}
	default:
		a.fail(errors.New("session snapshot has an unknown conversation phase"))
	}
}

func displayTitle(session agent.Session) string {
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

func (a *app) Close(ctx context.Context) {
	if a.closed {
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
		if a.conversation.Busy() || a.following || a.pendingCancel != nil {
			if entry, ok := a.queue.Next(a.session.ID); ok {
				if err := a.sendQueuedNow(entry.ID); err != nil {
					a.message(err.Error())
				}
			}
		}
		return
	}
	if name, arg, command := headless.Parse(message.Text); command {
		// A command acts on the staged composer context. Clear its command text but
		// put attachment elements back so /attachments and /detach can inspect or
		// mutate them without accidentally sending a user turn.
		a.restoreComposer(agent.Message{Attachments: message.Attachments})
		a.operations.Cancel(completionOperation)
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	if a.conversation.Busy() || a.following || a.pendingCancel != nil {
		a.enqueueFollowUp(message)
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
	a.startRun(message, "starting run")
}

func (a *app) message(label string) {
	if a.conversation.Phase() == agent.ConversationRunning {
		a.status.active(label)
		return
	}
	a.status.note(label)
}
