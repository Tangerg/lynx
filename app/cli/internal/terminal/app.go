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
	sendPrompt      keymap.Action = settings.ActionSend
	insertNewline   keymap.Action = settings.ActionNewline
	cancelRun       keymap.Action = settings.ActionCancelRun
	quitApp         keymap.Action = settings.ActionQuit
	commandPalette  keymap.Action = settings.ActionCommandPalette
	showSessions    keymap.Action = settings.ActionSessions
	searchHistory   keymap.Action = settings.ActionSearch
	manageQueue     keymap.Action = settings.ActionManageQueue
	cycleMode       keymap.Action = settings.ActionCycleMode
	toggleDetails   keymap.Action = settings.ActionToggleDetails
	historyPrevious keymap.Action = settings.ActionHistoryPrevious
	historyNext     keymap.Action = settings.ActionHistoryNext
	nextMatch       keymap.Action = settings.ActionNextMatch
	previousMatch   keymap.Action = settings.ActionPreviousMatch
	scrollPageUp    keymap.Action = settings.ActionScrollPageUp
	scrollPageDown  keymap.Action = settings.ActionScrollPageDown
	scrollTop       keymap.Action = settings.ActionScrollTop
	scrollBottom    keymap.Action = settings.ActionScrollBottom
	queueFollowUp   keymap.Action = "queue-follow-up"
	queueOrSendNext keymap.Action = "queue-or-send-next"
)

type app struct {
	ctx          context.Context
	loop         *program.InlineRuntime
	runtime      agent.Runtime
	session      agent.Session
	registry     *extensions.Registry
	plugins      *extensions.Kernel
	pluginIssues []extensions.SourceIssue
	state        *agent.Conversation
	operations   *operationOwner

	transcript  *conversationView
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
	queue       *promptqueue.Store

	review             *agent.Approval
	reviewChoice       string
	reviewForm         *headless.Form
	reviewPane         reviewPane
	reviewDialog       *kit.Dialog
	sessionPicker      *picker[agent.Session]
	sessionDialog      *kit.Dialog
	modelPicker        *picker[agent.Model]
	modelDialog        *kit.Dialog
	permissionPicker   *picker[agent.PermissionMode]
	permissionDialog   *kit.Dialog
	question           *agent.Question
	questionDialog     *kit.Dialog
	questionText       map[string]*string
	questionMulti      map[string]*[]string
	questionBool       map[string]*bool
	commandPicker      *picker[headless.Command]
	commandDialog      *kit.Dialog
	searchDialog       *kit.Dialog
	queueDialog        *headless.Dialog
	searchQuery        string
	attachments        *attachment.Resolver
	attachmentElements map[uint64]agent.Attachment
	history            promptHistory
	commandSeq         uint64
	commandOperations  map[uint64]commandOperation
	confirmation       pressConfirmation

	streamSeq             uint64
	dispatchingQueueEntry uint64
	startRequest          string
	pendingCancel         *agent.CancelRun
	following             bool
	stopClock             func()
	started               time.Time
	closed                bool
	syntax                highlight.Style
}

type appConfig struct {
	Context       context.Context
	Runtime       agent.Runtime
	Snapshot      agent.SessionSnapshot
	Registry      *extensions.Registry
	Plugins       *extensions.Kernel
	PluginIssues  []extensions.SourceIssue
	Attachments   *attachment.Resolver
	InitialPrompt string
	Settings      settings.Config
	Keys          *keymap.Map
	Queue         *promptqueue.Store
}

func newApp(loop *program.InlineRuntime, cfg appConfig) *app {
	ground := loop.Environment().Ground()
	theme := kit.Suited(ground)
	glyphs := kit.GlyphsFor(loop.Environment().Locale())
	syntax := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		syntax = "github"
	}
	a := &app{
		ctx: cfg.Context, loop: loop, runtime: cfg.Runtime, session: cfg.Snapshot.Session, registry: cfg.Registry,
		plugins: cfg.Plugins, pluginIssues: cfg.PluginIssues,
		state:              agent.NewConversation(),
		operations:         newOperationOwner(cfg.Context),
		transcript:         newConversationView(theme, glyphs, loop.Environment().Wheel(), syntax, cfg.Settings.UI.TranscriptRetain, cfg.Settings.UI.ToolDetails, loop.Clipboard()),
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
	}
	a.composer = kit.Composer{
		Theme: theme, Prompt: glyphs.Marker + " ",
		MaxRows: 6,
	}
	a.composer.Editor().Placeholder = "Ask lyra to inspect, explain, or change something"
	a.composer.Editor().Keys = cfg.Keys
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

	a.prompt = newPromptView(theme, glyphs, cfg.Keys, &a.composer, a.options)
	a.shell = newShellView(a.header, a.transcript, a.activity, a.queueView, a.status, a.prompt)
	a.wireTranscript(a.transcript)
	a.shell.Focus(true)
	a.stack.SetBase(a.shell)
	a.buildQueueDrawer(theme, glyphs, cfg.Keys)
	a.buildReview(theme, glyphs)
	a.buildSessionPicker(theme, glyphs)
	a.buildRuntimePickers(theme, glyphs)
	a.buildCommandPalette(theme, glyphs)
	a.buildSearchDialog(theme, glyphs)
	a.listenForSearch()
	loop.Session().SetTitle("lyra — " + displayTitle(cfg.Snapshot.Session))
	a.restore(cfg.Snapshot)
	return a
}

func (a *app) wireTranscript(transcript *conversationView) {
	a.prompt.SetTranscriptKeys(transcript.Keys())
	transcript.OnFocusChange(a.prompt.SetTranscriptFocused)
	transcript.OnSelection(a.prompt.SetTranscriptSelection)
	transcript.OnCopy(func(string) {
		if !a.state.Busy() && !a.following {
			a.status.note("copied selected transcript text")
		}
	})
}

func (a *app) buildSessionPicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.sessionPicker = newPicker(theme, glyphs, "search sessions",
		displayTitle,
		func(session agent.Session) string { return agoShort(session.UpdatedAt) + " · " + session.Workspace },
		func(session agent.Session) {
			a.sessionDialog.Dismiss()
			a.switchSession(session.ID)
		},
	)
	a.sessionDialog = kit.NewDialog(&a.stack, theme, glyphs, "Sessions", a.sessionPicker)
	a.sessionDialog.Panel().Where = layout.Placement{Width: 88, Height: 18, Margin: 1}
	a.sessionPicker.cancel = a.sessionDialog.Dismiss
}

func (a *app) restore(snapshot agent.SessionSnapshot) {
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

func presentSnapshot(view *conversationView, snapshot agent.SessionSnapshot, registry *extensions.Registry) error {
	for _, envelope := range snapshot.Events {
		if err := view.Apply(envelope.Event, registry); err != nil {
			return fmt.Errorf("restore transcript at cursor %d: %w", envelope.Cursor, err)
		}
	}
	return nil
}

func (a *app) restoreActivity(snapshot agent.SessionSnapshot) {
	a.activity.Set(a.state.Plan())
	a.header.SetUsage(a.state.Usage())
	a.prompt.SetBusy(a.state.Busy())
	switch a.state.Phase() {
	case agent.PhaseWaiting:
		a.openInteraction(a.state.Interaction())
		a.status.note("waiting for your answer")
	case agent.PhaseRunning:
		if snapshot.Active == nil {
			a.fail(errors.New("session snapshot has a running conversation without an active run"))
			return
		}
		a.status.active("reconnecting")
		a.follow(func(context.Context) (subscription, error) {
			return subscription{runID: snapshot.Active.ID, after: snapshot.Cursor}, nil
		})
	case agent.PhaseIdle:
		if a.state.Outcome().Status != "" {
			a.status.settled(a.state.Outcome(), a.state.Usage())
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
		if a.state.Busy() || a.following || a.pendingCancel != nil {
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
	if a.state.Busy() || a.following || a.pendingCancel != nil {
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
	if a.state.Phase() == agent.PhaseRunning {
		a.status.active(label)
		return
	}
	a.status.note(label)
}
