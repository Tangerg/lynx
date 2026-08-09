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

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

const (
	animationRate  = 100 * time.Millisecond
	controlTimeout = 5 * time.Second
)

const (
	sendPrompt     keymap.Action = settings.ActionSend
	cancelRun      keymap.Action = settings.ActionCancelRun
	quitApp        keymap.Action = settings.ActionQuit
	commandPalette keymap.Action = settings.ActionCommandPalette
	showSessions   keymap.Action = settings.ActionSessions
	searchHistory  keymap.Action = settings.ActionSearch
	cycleMode      keymap.Action = settings.ActionCycleMode
	toggleDetails  keymap.Action = settings.ActionToggleDetails
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
	Runtime   runtime
	Session   string
	Workspace string
	Prompt    string
	Plugins   []extensions.Plugin
	Host      program.Host
	Settings  settings.Settings
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Runtime == nil {
		return errors.New("session: a runtime is required")
	}
	if cfg.Settings.Model == "" {
		cfg.Settings = settings.Default()
	}
	if err := cfg.Settings.Validate(); err != nil {
		return fmt.Errorf("session settings: %w", err)
	}
	keys, err := configuredKeys(cfg.Settings)
	if err != nil {
		return err
	}
	opened, err := open(ctx, cfg.Runtime, cfg.Session, cfg.Workspace)
	if err != nil {
		return err
	}

	registry := new(extensions.Registry)
	loaded, err := loadPlugins(registry, append([]extensions.Plugin{builtinPlugin()}, cfg.Plugins...))
	if err != nil {
		return err
	}
	defer disposePlugins(loaded)

	var active *app
	err = program.Run(ctx, program.Config{
		Inline: func(loop *program.InlineRuntime) program.Component {
			active = newApp(ctx, loop, cfg.Runtime, opened, registry, cfg.Prompt, cfg.Settings, keys)
			return headless.NewRoot(active)
		},
		Terminal: term.Options{Probe: true, Mouse: cfg.Settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	})
	if active != nil {
		active.Close()
	}
	return err
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
		return client.SessionSnapshot{Session: created}, nil
	}
	return rt.GetSession(ctx, id)
}

func loadPlugins(registry *extensions.Registry, plugins []extensions.Plugin) ([]*extensions.Loaded, error) {
	loaded := make([]*extensions.Loaded, 0, len(plugins))
	for _, plugin := range plugins {
		item, err := extensions.Load(registry, plugin)
		if err != nil {
			disposePlugins(loaded)
			return nil, err
		}
		loaded = append(loaded, item)
	}
	return loaded, nil
}

func disposePlugins(loaded []*extensions.Loaded) {
	for i := len(loaded) - 1; i >= 0; i-- {
		loaded[i].Dispose()
	}
}

type app struct {
	ctx      context.Context
	loop     *program.InlineRuntime
	backend  runtime
	session  client.Session
	registry *extensions.Registry
	state    *client.Conversation

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

	review           *client.Approval
	reviewChoice     string
	reviewForm       *headless.Form
	reviewPane       reviewPane
	reviewDialog     *kit.Dialog
	sessionPicker    *picker[client.Session]
	sessionDialog    *kit.Dialog
	modelPicker      *picker[client.Model]
	modelDialog      *kit.Dialog
	permissionPicker *picker[client.PermissionMode]
	permissionDialog *kit.Dialog
	question         *client.Question
	questionDialog   *kit.Dialog
	questionText     map[string]*string
	questionMulti    map[string]*[]string
	questionBool     map[string]*bool
	commandPicker    *picker[headless.Command]
	commandDialog    *kit.Dialog
	searchDialog     *kit.Dialog
	searchQuery      string

	streamCancel context.CancelFunc
	streamSeq    uint64
	switchSeq    uint64
	following    bool
	stopClock    func()
	started      time.Time
	closed       bool
	syntax       highlight.Style
}

func newApp(ctx context.Context, loop *program.InlineRuntime, backend runtime, opened client.SessionSnapshot, registry *extensions.Registry, prompt string, configured settings.Settings, keys *keymap.Map) *app {
	ground := loop.Environment().Ground()
	theme := kit.Suited(ground)
	glyphs := kit.GlyphsFor(os.LookupEnv)
	syntax := highlight.Style("github-dark")
	if !ground.BG.Default() && !ground.BG.RGB().Dark() {
		syntax = "github"
	}
	a := &app{
		ctx: ctx, loop: loop, backend: backend, session: opened.Session, registry: registry,
		state:      client.NewConversation(),
		transcript: newConversationView(theme, glyphs, loop.Environment().Wheel(), syntax, configured.UI.TranscriptRetain),
		workflow:   newWorkflowView(theme, glyphs),
		status:     statusView{theme: theme, glyphs: glyphs, doing: "ready", options: configured.RunOptions()},
		settings:   configured.Clone(),
		options:    configured.RunOptions(),
		syntax:     syntax,
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
		func(session client.Session) string { return displayTitle(session) },
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
	if err := a.state.Restore(snapshot.Events); err != nil {
		a.fail(err)
		return
	}
	for _, envelope := range snapshot.Events {
		if err := a.transcript.Apply(envelope.Event, a.registry); err != nil {
			a.fail(err)
			return
		}
	}
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
	default:
		if a.state.Outcome().Status != "" {
			a.status.settled(a.state.Outcome(), a.state.Usage())
		}
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
	var action keymap.Action
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ = a.composer.Editor().Keys.Action(key.Chord())
		switch action {
		case quitApp:
			a.loop.Quit()
			return true
		case cancelRun:
			a.cancel()
			return true
		}
	}
	if !a.stack.Empty() {
		return a.stack.Handle(event)
	}
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
		a.transcript.ToggleDetails()
		a.message(a.transcript.DetailsLabel())
		return true
	}
	if a.completion.Handle(event) {
		return true
	}
	if key, ok := event.(input.Key); ok && key.Down() && key.Code == input.Enter && key.Mods == 0 {
		a.submit()
		return true
	}
	handled := a.stack.Handle(event)
	if handled {
		a.refreshCompletion()
	}
	return handled
}

func (a *app) Close() {
	if a == nil || a.closed {
		return
	}
	a.closed = true
	a.dropStream()
	a.transcript.Close()
	if a.stopClock != nil {
		a.stopClock()
		a.stopClock = nil
	}
}

func (a *app) submit() {
	line := strings.TrimSpace(a.composer.Text())
	if line == "" {
		return
	}
	if name, arg, command := headless.Parse(line); command {
		a.composer.Reset()
		a.completion.Dismiss()
		a.runCommand(name, arg)
		return
	}
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run first")
		return
	}
	a.composer.Reset()
	a.completion.Dismiss()
	a.start(line)
}

func (a *app) start(prompt string) {
	a.state.Starting()
	a.workflow.Reset()
	a.status.active("starting mock runtime")
	a.started = time.Now()
	a.syncAnimation()
	a.follow(func(ctx context.Context) (subscription, error) {
		run, err := a.backend.StartRun(ctx, client.StartRun{SessionID: a.session.ID, Message: client.Message{Text: prompt}, Options: a.options})
		if err != nil {
			return subscription{}, err
		}
		return subscription{runID: run.ID, after: run.StartedAfter}, nil
	})
}

type subscription struct {
	runID string
	after client.Cursor
}

func (a *app) follow(open func(context.Context) (subscription, error)) {
	a.dropStream()
	sequence := a.streamSeq
	ctx, cancel := context.WithCancel(a.ctx)
	a.streamCancel = cancel
	a.following = true
	dispatcher := a.loop.Dispatcher()

	go func() {
		defer cancel()
		sub, err := open(ctx)
		if err != nil {
			_ = post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.fail(err)
				}
			})
			return
		}
		stream, err := a.backend.FollowRun(ctx, client.FollowRun{RunID: sub.runID, After: sub.after})
		if err != nil {
			_ = post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.fail(err)
				}
			})
			return
		}
		if stream == nil {
			_ = post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.fail(errors.New("runtime returned a nil event stream"))
				}
			})
			return
		}
		for envelope, streamErr := range stream {
			if streamErr != nil {
				if !errors.Is(streamErr, context.Canceled) {
					_ = post(ctx, dispatcher, func() {
						if a.streamSeq == sequence {
							a.fail(streamErr)
						}
					})
				}
				return
			}
			if err := post(ctx, dispatcher, func() {
				if a.streamSeq == sequence {
					a.apply(envelope)
				}
			}); err != nil {
				return
			}
		}
		_ = post(context.WithoutCancel(ctx), dispatcher, func() {
			if a.streamSeq != sequence {
				return
			}
			a.streamCancel = nil
			a.following = false
			if a.state.Phase() == client.Running {
				a.fail(errors.New("runtime stream ended without parking or finishing the run"))
			}
		})
	}()
}

func post(ctx context.Context, dispatcher program.Dispatcher, fn func()) error {
	done := make(chan struct{}, 1)
	dispatcher.Post(func() {
		fn()
		done <- struct{}{}
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-dispatcher.Done():
		return program.ErrStopped
	}
}

func (a *app) apply(envelope client.Envelope) {
	result, err := a.state.ApplyEnvelope(envelope)
	if err != nil {
		a.fail(fmt.Errorf("apply runtime event %T at cursor %d: %w", envelope.Event, envelope.Cursor, err))
		return
	}
	if !result.Applied {
		return
	}
	event := envelope.Event
	if err := a.transcript.Apply(event, a.registry); err != nil {
		a.fail(err)
		return
	}
	switch event.(type) {
	case client.RunStarted:
		a.status.active("working")
	case client.BlockStarted:
		if started := event.(client.BlockStarted); started.Block.Kind == client.BlockTool && started.Block.Tool != nil {
			a.status.active(started.Block.Tool.Summary)
		}
	case client.BlockCompleted:
		if completed := event.(client.BlockCompleted); completed.Block.Kind == client.BlockTool {
			a.status.active("working")
		}
	case client.PlanChanged:
		a.workflow.Set(a.state.Plan())
	case client.RunInterrupted:
		a.openInteraction(a.state.Interaction())
		a.status.note("waiting for your answer")
	case client.RunFinished:
		a.following = false
		a.status.settled(a.state.Outcome(), a.state.Usage())
		if a.settings.UI.Notifications {
			a.loop.Session().Notify("lyra mock run completed")
		}
	}
	a.transcript.Retain(a.loop)
	a.syncAnimation()
}

func (a *app) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.following = false
	a.state.Failed(err)
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.state.Outcome(), a.state.Usage())
	a.syncAnimation()
}

func (a *app) cancel() {
	if a.review != nil {
		a.answerReview("deny")
		return
	}
	if a.question != nil {
		a.answerQuestion(true)
		return
	}
	if !a.state.Busy() && !a.following {
		a.loop.Quit()
		return
	}
	a.status.doing = "cancelling"
	runID := a.state.RunID()
	if runID == "" {
		a.dropStream()
		a.following = false
		_ = a.state.Apply(client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
		a.status.settled(a.state.Outcome(), a.state.Usage())
		a.syncAnimation()
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), controlTimeout)
		defer cancel()
		if err := a.backend.CancelRun(ctx, runID); err != nil {
			a.loop.Dispatcher().Post(func() { a.fail(err) })
		}
	}()
}

func (a *app) dropStream() {
	a.streamSeq++
	if a.streamCancel != nil {
		a.streamCancel()
		a.streamCancel = nil
	}
}

func (a *app) syncAnimation() {
	running := a.state.Phase() == client.Running
	switch {
	case running && a.stopClock == nil:
		a.stopClock = a.loop.Every(animationRate, func() {
			a.status.tick(time.Since(a.started))
		})
	case !running && a.stopClock != nil:
		a.stopClock()
		a.stopClock = nil
	}
	state := term.Progress{}
	if running {
		state.State = term.ProgressIndeterminate
	}
	a.loop.Session().SetProgress(state)
}
