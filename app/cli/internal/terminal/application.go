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
	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/authoringcontext"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/codebase"
	"github.com/Tangerg/lynx/app/cli/internal/diagnostictool"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/feedback"
	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/hookpolicy"
	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/schedule"
	"github.com/Tangerg/lynx/app/cli/internal/sessionartifact"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/skills"
	"github.com/Tangerg/lynx/app/cli/internal/usage"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
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
	openReader       keymap.Action = "open reader"
	historyPrevious  keymap.Action = settings.ActionHistoryPrevious
	historyNext      keymap.Action = settings.ActionHistoryNext
	nextMatch        keymap.Action = settings.ActionNextMatch
	previousMatch    keymap.Action = settings.ActionPreviousMatch
	scrollPageUp     keymap.Action = settings.ActionScrollPageUp
	scrollPageDown   keymap.Action = settings.ActionScrollPageDown
	scrollTop        keymap.Action = settings.ActionScrollTop
	scrollBottom     keymap.Action = settings.ActionScrollBottom
	editPrompt       keymap.Action = settings.ActionExternalEditor
	queueFollowUp    keymap.Action = "queue-follow-up"
	queueOrSendNext  keymap.Action = "queue-or-send-next"
)

type app struct {
	ctx              context.Context
	loop             *program.Runtime
	runtime          agent.Runtime
	workspaces       workspace.Service
	changes          changefeed.Source
	transfers        sessiontransfer.Service
	usage            usage.Service
	modelConfig      modelconfig.Service
	goals            goal.Service
	skills           skills.Service
	mcp              mcp.Service
	schedules        schedule.Service
	agentMemory      agentmemory.Service
	knowledge        knowledge.Service
	diagnosticTools  diagnostictool.Service
	codebase         codebase.Service
	authoringContext authoringcontext.Service
	hooks            hookpolicy.Service
	feedback         feedback.Service
	runtimeProfile   *runtimeprofile.Profile
	artifacts        sessionartifact.Store
	session          agent.Session
	registry         *extensions.Registry
	pluginHost       *extensions.Host
	pluginIssues     []extensions.SourceIssue
	conversation     *agent.Conversation
	operations       *operationOwner

	transcript     *transcriptView
	brand          *brandBanner
	header         *sessionHeader
	activity       *activityView
	queueView      *queueView
	queueDrawer    *queueDrawer
	status         *statusView
	settings       settings.Config
	options        agent.RunOptions
	composer       kit.Composer
	prompt         *promptView
	commands       commandCatalog
	completion     headless.Completion
	completionGate completionGate
	shell          *shellView
	stack          headless.Stack
	queue          *promptqueue.Queue
	workbench      *workbench.Store
	editor         promptEditor

	approval            *agent.Approval
	approvalChoice      string
	approvalReason      string
	approvalForm        *headless.Form
	approvalPane        approvalPane
	approvalDialog      *kit.Dialog
	sessionCenter       *sessionCenterPane
	sessionDialog       *kit.Dialog
	sessionRenameDialog *kit.Dialog
	sessionDeleteDialog *kit.Dialog
	confirmationDialog  *kit.Dialog
	workspacePicker     *picker[workspaceChoice]
	workspaceDialog     *kit.Dialog
	timeline            *timelinePane
	timelineDialog      *kit.Dialog
	modelPicker         *picker[agent.Model]
	modelDialog         *kit.Dialog
	approvalModePicker  *picker[agent.ApprovalMode]
	approvalModeDialog  *kit.Dialog
	providerDialog      *kit.Dialog
	mcpDialog           *kit.Dialog
	scheduleDialog      *kit.Dialog
	contextEditorDialog *kit.Dialog
	questionnaire       *questionnaire
	questionDialog      *kit.Dialog
	interactionReview   *interactionReview
	reviewDialog        *kit.Dialog
	commandPicker       *picker[commandPaletteItem]
	commandDialog       *kit.Dialog
	shortcutDialog      *kit.Dialog
	shortcutViewport    *headless.Viewport
	searchDialog        *kit.Dialog
	reader              *readerPane
	readerDialog        *kit.Dialog
	readerSearchDialog  *kit.Dialog
	readerSearchQuery   string
	workspaceReader     workspaceReaderMode
	runtimeReader       runtimeReaderMode
	mcpToolServer       string
	mcpAuthorizationID  string
	queueDialog         *headless.Dialog
	searchQuery         string
	attachments         *attachment.Resolver
	attachmentElements  map[uint64]agent.Attachment
	history             promptHistory
	workbenchProblem    string
	sessionInvalidated  bool
	commandSeq          uint64
	commandOperations   map[uint64]commandOperation
	confirmation        pressConfirmation
	applicationKeys     *keymap.Map
	globalKeys          *keymap.Map
	applicationMatcher  keymap.Matcher
	globalMatcher       keymap.Matcher
	attention           attentionCenter

	streamSeq              uint64
	dispatchingQueueEntry  uint64
	pendingCancel          *agent.CancelRun
	sessionDraftTransition *sessionDraftTransition
	following              bool
	stopClock              func()
	executionClock         activeDurationClock
	closed                 bool
	syntax                 highlight.Renderer
}

type appConfig struct {
	context          context.Context
	runtime          agent.Runtime
	runtimeProfile   *runtimeprofile.Profile
	workspaces       workspace.Service
	changes          changefeed.Source
	transfers        sessiontransfer.Service
	usage            usage.Service
	modelConfig      modelconfig.Service
	goals            goal.Service
	skills           skills.Service
	mcp              mcp.Service
	schedules        schedule.Service
	agentMemory      agentmemory.Service
	knowledge        knowledge.Service
	diagnosticTools  diagnostictool.Service
	codebase         codebase.Service
	authoringContext authoringcontext.Service
	hooks            hookpolicy.Service
	feedback         feedback.Service
	clientVersion    string
	snapshot         agent.SessionSnapshot
	registry         *extensions.Registry
	pluginHost       *extensions.Host
	pluginIssues     []extensions.SourceIssue
	attachments      *attachment.Resolver
	initialDraft     agent.Message
	settings         settings.Config
	keyBindings      keyBindings
	queue            *promptqueue.Queue
	workbench        *workbench.Store
	editor           promptEditor
}

type terminalAppearance struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	syntax highlight.Renderer
}

func newTerminalAppearance(loop *program.Runtime) terminalAppearance {
	ground := loop.Environment().Ground()
	style := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		style = "github"
	}
	return terminalAppearance{
		theme: kit.Suited(ground), glyphs: kit.GlyphsFor(loop.Environment().Locale()), syntax: highlight.New(style),
	}
}

func newApp(loop *program.Runtime, cfg appConfig) *app {
	cfg.keyBindings.setResolver(loop.After)
	appearance := newTerminalAppearance(loop)
	transcript := newTranscriptView(appearance.theme, appearance.glyphs, loop.Environment().Wheel(), appearance.syntax, cfg.settings.UI.TranscriptRetain, cfg.settings.UI.ToolDetails, loop.Clipboard())
	brand := newBrandBanner(appearance.theme, appearance.glyphs, cfg.clientVersion, cfg.snapshot.Session, cfg.settings.RunOptions())
	transcript.SetEntrance(brand)
	a := &app{
		ctx: cfg.context, loop: loop, runtime: cfg.runtime, workspaces: cfg.workspaces,
		runtimeProfile: cfg.runtimeProfile,
		changes:        cfg.changes, transfers: cfg.transfers, usage: cfg.usage, modelConfig: cfg.modelConfig,
		goals: cfg.goals, skills: cfg.skills, mcp: cfg.mcp, schedules: cfg.schedules,
		agentMemory: cfg.agentMemory, knowledge: cfg.knowledge,
		diagnosticTools: cfg.diagnosticTools, codebase: cfg.codebase,
		authoringContext: cfg.authoringContext, hooks: cfg.hooks, feedback: cfg.feedback,
		session: cfg.snapshot.Session, registry: cfg.registry,
		pluginHost: cfg.pluginHost, pluginIssues: cfg.pluginIssues,
		conversation:       agent.NewConversation(),
		operations:         newOperationOwner(cfg.context),
		transcript:         transcript,
		brand:              brand,
		header:             newSessionHeader(appearance.theme, appearance.glyphs, cfg.snapshot.Session),
		activity:           newActivityView(appearance.theme, appearance.glyphs),
		queueView:          newQueueView(appearance.theme, appearance.glyphs),
		status:             newStatusView(appearance.theme, appearance.glyphs, cfg.settings.RunOptions()),
		queue:              cfg.queue,
		workbench:          cfg.workbench,
		editor:             cfg.editor,
		settings:           cfg.settings.Clone(),
		options:            cfg.settings.RunOptions(),
		syntax:             appearance.syntax,
		attachments:        cfg.attachments,
		attachmentElements: make(map[uint64]agent.Attachment),
		commandOperations:  make(map[uint64]commandOperation),
		commands:           newCommandCatalog(),
		attention:          newAttentionCenter(),
		applicationKeys:    cfg.keyBindings.application,
		globalKeys:         cfg.keyBindings.global,
	}
	a.transcript.images = newTerminalImagePresenter(loop.Images())
	a.configureComposer(appearance, cfg.keyBindings.editor, cfg.initialDraft)
	a.configureCompletion(appearance)
	a.registerCommands()
	a.buildInterface(appearance, cfg.keyBindings.editor)
	a.restore(cfg.snapshot)
	a.persistDraft()
	a.followRuntimeChanges()
	return a
}

func (a *app) configureComposer(appearance terminalAppearance, keys *keymap.Map, initial agent.Message) {
	a.composer = kit.Composer{
		Theme: appearance.theme, Prompt: appearance.glyphs.Marker + " ",
		MaxRows: 6,
	}
	a.composer.Editor().Placeholder = "Ask lyra to inspect, explain, or change something"
	a.composer.Editor().Keys = keys
	a.composer.Editor().Clipboard = a.loop.Clipboard()
	if a.workbench != nil {
		a.history.Load(a.workbench.History())
	}
	a.restoreComposer(initial)
}

func (a *app) configureCompletion(appearance terminalAppearance) {
	completionKeys := headless.DefaultCompletionKeys()
	completionKeys.Bind(headless.Accept, input.Chord{Code: input.Enter})
	a.completion = headless.Completion{
		Look: appearance.theme.Look(appearance.glyphs), Keys: completionKeys,
		Accept: func(candidate headless.Candidate, token headless.Token) {
			// Acceptance closes both halves of completion: Oolong has already
			// dismissed the list, while the application must retire its async
			// producer so an older file lookup cannot reopen the accepted token.
			a.operations.Cancel(completionOperation)
			if token.Trigger.Prefix == "@" {
				a.acceptAttachmentCompletion(candidate.Text, token)
				return
			}
			a.completionGate.Reset()
			a.composer.Editor().Replace(token.Start, token.End, candidate.Text)
		},
	}
}

func (a *app) buildInterface(appearance terminalAppearance, editorKeys *keymap.Map) {
	theme, glyphs := appearance.theme, appearance.glyphs
	a.prompt = newPromptView(theme, glyphs, editorKeys, &a.composer, a.options)
	a.shell = newShellView(a.header, a.transcript, a.activity, a.queueView, a.status, a.prompt)
	a.wireTranscript(a.transcript)
	a.shell.Focus(true)
	a.stack.SetBase(a.shell)
	a.buildQueueDrawer(theme, glyphs, editorKeys)
	a.buildApprovalDialog(theme, glyphs)
	a.buildSessionPicker(theme, glyphs)
	a.buildWorkspacePicker(theme, glyphs)
	a.buildTimeline(theme, glyphs)
	a.buildRuntimePickers(theme, glyphs)
	a.buildCommandPalette(theme, glyphs)
	a.buildShortcutDialog(theme, glyphs, editorKeys)
	a.buildSearchDialog(theme, glyphs)
	a.buildReader(theme, glyphs)
	a.listenForSearch()
	a.setWindowTitle()
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
	a.sessionCenter = newSessionCenterPane(theme, glyphs, func(session agent.Session) {
		a.sessionDialog.Dismiss()
		a.switchSession(session.ID)
	})
	a.sessionCenter.loadMore = a.loadMoreSessions
	a.sessionCenter.toggleFavorite = a.toggleSessionFavorite
	a.sessionCenter.rename = a.openSessionRename
	a.sessionCenter.delete = a.openSessionDelete
	a.sessionDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Sessions · Center", Body: a.sessionCenter,
		Where: layout.Placement{Width: 96, Height: 24},
	})
	a.sessionCenter.picker.cancel = a.sessionDialog.Dismiss
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
	view.SetRuns(snapshot.Runs)
	for _, block := range snapshot.Transcript {
		var event agent.Event = agent.BlockCompleted{Block: block}
		if block.Status == agent.BlockStatusRunning {
			event = agent.BlockStarted{Block: block}
		}
		if err := view.Apply(event, registry); err != nil {
			return fmt.Errorf("restore transcript block %s: %w", block.ID, err)
		}
	}
	view.SealToolGroups()
	return nil
}

type sessionProjection struct {
	conversation *agent.Conversation
	transcript   *transcriptView
}

func (projection sessionProjection) close() {
	if projection.transcript != nil {
		projection.transcript.Close()
	}
}

func (a *app) projectSession(snapshot agent.SessionSnapshot, attached *agent.SegmentStream) (sessionProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return sessionProjection{}, err
	}
	conversation := agent.NewConversation()
	var err error
	if active, ok := snapshot.ActiveRun(); attached != nil && ok && active.Status == agent.RunStatusRunning {
		err = conversation.RestoreAttachedSnapshot(snapshot, *attached)
	} else {
		err = conversation.RestoreSnapshot(snapshot)
	}
	if err != nil {
		return sessionProjection{}, err
	}
	transcript := a.newTranscript()
	if err := presentSnapshot(transcript, snapshot, a.registry); err != nil {
		transcript.Close()
		return sessionProjection{}, err
	}
	return sessionProjection{conversation: conversation, transcript: transcript}, nil
}

func (a *app) newTranscript() *transcriptView {
	transcript := newTranscriptView(
		a.transcript.theme, a.transcript.glyphs, a.transcript.wheel, a.syntax,
		a.settings.UI.TranscriptRetain, a.transcript.details, a.transcript.clipboard,
	)
	transcript.images = a.transcript.images
	return transcript
}

// reconcileRunSnapshot atomically replaces the in-memory projection after a
// segment can no longer be replayed. It deliberately keeps the current stream
// operation alive: runrecovery already attached the replacement stream before
// taking this snapshot, so canceling that operation here would reopen a gap.
func (a *app) reconcileRunSnapshot(snapshot agent.SessionSnapshot, stream agent.SegmentStream) error {
	if snapshot.Session.ID != a.session.ID {
		return fmt.Errorf("reconcile run snapshot: session %s does not match %s", snapshot.Session.ID, a.session.ID)
	}
	projection, err := a.projectSession(snapshot, &stream)
	if err != nil {
		return fmt.Errorf("reconcile run snapshot: %w", err)
	}

	previousTranscript := a.transcript
	a.setActiveSession(snapshot.Session)
	a.conversation = projection.conversation
	a.transcript = projection.transcript
	a.wireTranscript(projection.transcript)
	a.shell.SetTranscript(projection.transcript)
	a.header.SetUsage(projection.conversation.Usage())
	a.activity.Set(projection.conversation.Plan())
	a.prompt.SetBusy(projection.conversation.Busy())
	previousTranscript.Close()
	a.listenForSearch()

	switch projection.conversation.Phase() {
	case agent.ConversationRunning:
		a.following = true
		a.executionClock.start(projection.conversation.Usage().Duration, time.Now())
		a.status.active("reconnected")
	case agent.ConversationWaiting:
		a.following = false
		a.openInteractions(projection.conversation.Interactions())
		a.status.note("waiting for your answers")
	case agent.ConversationIdle:
		a.following = false
		if projection.conversation.Outcome().Status != "" {
			a.status.settled(projection.conversation.Outcome(), projection.conversation.Usage())
		}
		if a.drainQueue() {
			return nil
		}
		if projection.conversation.Outcome().Status != "" {
			a.raiseAttention(outcomeAttention(projection.conversation.Outcome()))
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

func (a *app) setActiveSession(session agent.Session) {
	a.session = session
	a.header.SetSession(session)
	a.brand.SetSession(session)
	a.setWindowTitle()
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
	a.persistDraft()
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
	if a.reader != nil {
		a.reader.Shutdown()
	}
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
		a.sendNextQueuedIfBusy()
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
	a.dispatchPrompt(message)
}

// dispatchPrompt owns the single path from an authored message to either the
// active run or its durable follow-up queue. Callers such as recipe expansion
// cannot bypass session-change exclusion, prompt history, or composer cleanup.
func (a *app) dispatchPrompt(message agent.Message) {
	if err := a.validateMessageCapabilities(message); err != nil {
		a.message(err.Error())
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
	a.rememberPrompt(message)
	a.resetComposer()
	a.operations.Cancel(completionOperation)
	a.completion.Dismiss()
	a.startRun(message, "starting run")
}

func (a *app) sendNextQueuedIfBusy() {
	if !a.conversation.Busy() && !a.following && a.pendingCancel == nil {
		return
	}
	entry, ok := a.queue.Next(a.session.ID)
	if !ok {
		return
	}
	if err := a.sendQueuedNow(entry.ID); err != nil {
		a.message(err.Error())
	}
}

func (a *app) message(label string) {
	if a.conversation.Phase() == agent.ConversationRunning {
		a.status.active(label)
		return
	}
	a.status.note(label)
}
