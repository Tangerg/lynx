// Package terminal is the interactive adapter for the Lyra runtime. It owns
// oolong state and translates user intent into the runtime port; neither the
// domain model nor a runtime adapter imports this package.
package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

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
	"github.com/Tangerg/lynx/app/cli/internal/session"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/skills"
	"github.com/Tangerg/lynx/app/cli/internal/usage"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

// Config describes one terminal application instance.
type Config struct {
	Runtime          agent.Runtime
	RuntimeProfile   *runtimeprofile.Profile
	Workspaces       workspace.Service
	Changes          changefeed.Source
	Transfers        sessiontransfer.Service
	Usage            usage.Service
	ModelConfig      modelconfig.Service
	Goals            goal.Service
	Skills           skills.Service
	MCP              mcp.Service
	Schedules        schedule.Service
	AgentMemory      agentmemory.Service
	Knowledge        knowledge.Service
	DiagnosticTools  diagnostictool.Service
	Codebase         codebase.Service
	AuthoringContext authoringcontext.Service
	Hooks            hookpolicy.Service
	Feedback         feedback.Service
	SessionID        string
	Workspace        string
	InitialPrompt    string
	Plugins          []extensions.Plugin
	PluginSources    []extensions.Source
	Host             program.Host
	Settings         *settings.Config
	StateDirectory   string
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) (runErr error) {
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}

	registry := new(extensions.Registry)
	extensionHost, err := extensions.NewHost(registry)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, extensionHost.Close()) }()
	sources := make([]extensions.Source, 0, 1+len(cfg.PluginSources))
	sources = append(sources, extensions.StaticSource{
		Name: "terminal", Plugins: append([]extensions.Plugin{builtinPlugin()}, cfg.Plugins...),
	})
	sources = append(sources, cfg.PluginSources...)
	discovered, err := extensions.Discover(ctx, sources...)
	if err != nil {
		return err
	}
	results, err := extensionHost.Activate(discovered.Plugins)
	if err != nil {
		return err
	}
	if err := requireLoadedPlugin(results, "terminal.core"); err != nil {
		return err
	}

	var active *app
	queue := promptqueue.New()
	err = program.Run(ctx, program.Config{
		Root: func(loop *program.Runtime) program.Component {
			active = newApp(loop, appConfig{
				context: ctx, runtime: cfg.Runtime, snapshot: prepared.opened,
				runtimeProfile: prepared.runtimeProfile,
				workspaces:     cfg.Workspaces, changes: cfg.Changes,
				transfers: cfg.Transfers, usage: cfg.Usage, modelConfig: cfg.ModelConfig,
				goals: cfg.Goals, skills: cfg.Skills, mcp: cfg.MCP, schedules: cfg.Schedules,
				agentMemory: cfg.AgentMemory, knowledge: cfg.Knowledge,
				diagnosticTools: cfg.DiagnosticTools, codebase: cfg.Codebase,
				authoringContext: cfg.AuthoringContext, hooks: cfg.Hooks, feedback: cfg.Feedback,
				registry: registry, pluginHost: extensionHost, pluginIssues: discovered.Issues,
				attachments: prepared.attachments,
				settings:    prepared.settings, keyBindings: prepared.keyBindings, queue: queue,
				workbench: prepared.workbench, initialDraft: prepared.draft, editor: prepared.editor,
			})
			return headless.NewRoot(active)
		},
		Terminal: term.Config{Probe: true, Mouse: prepared.settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	})
	if active != nil {
		active.Close(ctx)
	}
	return err
}

type preparedSession struct {
	opened         agent.SessionSnapshot
	runtimeProfile *runtimeprofile.Profile
	attachments    *attachment.Resolver
	keyBindings    keyBindings
	settings       settings.Config
	workbench      *workbench.Store
	draft          agent.Message
	editor         *draftEditor
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	if cfg.Runtime == nil {
		return preparedSession{}, errors.New("session: a runtime is required")
	}
	var profile *runtimeprofile.Profile
	if cfg.RuntimeProfile != nil {
		cloned := cfg.RuntimeProfile.Clone()
		if err := cloned.Validate(); err != nil {
			return preparedSession{}, fmt.Errorf("session runtime profile: %w", err)
		}
		profile = &cloned
	}
	configured := settings.Default()
	if cfg.Settings != nil {
		configured = cfg.Settings.Clone()
	}
	if err := configured.Validate(); err != nil {
		return preparedSession{}, fmt.Errorf("session settings: %w", err)
	}
	bindings, err := configuredKeyBindings(configured)
	if err != nil {
		return preparedSession{}, err
	}
	opened, err := session.Open(ctx, cfg.Runtime, cfg.SessionID, cfg.Workspace)
	if err != nil {
		return preparedSession{}, err
	}
	attachments, err := attachment.New(opened.Session.Workspace.Path)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session attachments: %w", err)
	}
	authoring, err := workbench.Open(cfg.StateDirectory, workbench.Config{})
	if err != nil {
		return preparedSession{}, fmt.Errorf("open CLI workbench: %w", err)
	}
	if err := authoring.RememberWorkspace(opened.Session.Workspace.Path); err != nil {
		return preparedSession{}, fmt.Errorf("remember workspace: %w", err)
	}
	draft, _, err := authoring.Draft(opened.Session.ID)
	if err != nil {
		return preparedSession{}, fmt.Errorf("load session draft: %w", err)
	}
	if cfg.InitialPrompt != "" {
		draft = agent.Message{Text: cfg.InitialPrompt}
	}
	editor, err := configuredDraftEditor()
	if err != nil {
		return preparedSession{}, err
	}
	return preparedSession{
		opened: opened, runtimeProfile: profile, attachments: attachments, keyBindings: bindings, settings: configured,
		workbench: authoring, draft: draft, editor: editor,
	}, nil
}

func requireLoadedPlugin(results []extensions.LifecycleResult, id string) error {
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
