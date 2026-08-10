// Package terminal is the interactive adapter for the Lyra runtime. It owns
// oolong state and translates user intent into the runtime port; neither the
// domain model nor a runtime adapter imports this package.
package terminal

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/lynx/app/cli/internal/attachment"
	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/promptqueue"
	"github.com/Tangerg/lynx/app/cli/internal/session"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

// Config describes one terminal application instance.
type Config struct {
	Runtime       client.Runtime
	SessionID     string
	Workspace     string
	InitialPrompt string
	Plugins       []extensions.Plugin
	PluginSources []extensions.Source
	Host          program.Host
	Settings      *settings.Config
}

// Run opens and owns the terminal interface until the user leaves.
func Run(ctx context.Context, cfg Config) (runErr error) {
	prepared, err := prepareSession(ctx, cfg)
	if err != nil {
		return err
	}

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
	queue := promptqueue.New()
	err = program.Run(ctx, program.Config{
		Inline: func(loop *program.InlineRuntime) program.Component {
			active = newApp(loop, appConfig{
				Context: ctx, Runtime: cfg.Runtime, Snapshot: prepared.opened,
				Registry: registry, Plugins: kernel, PluginIssues: discovered.Issues,
				Attachments: prepared.attachments, InitialPrompt: cfg.InitialPrompt,
				Settings: prepared.settings, Keys: prepared.keys, Queue: queue,
			})
			return headless.NewRoot(active)
		},
		Terminal: term.Options{Probe: true, Mouse: prepared.settings.UI.Mouse, Focus: true, Keyboard: term.KeyboardCompatible},
		Host:     cfg.Host,
	})
	if active != nil {
		active.Close(ctx)
	}
	return err
}

type preparedSession struct {
	opened      client.SessionSnapshot
	attachments *attachment.Resolver
	keys        *keymap.Map
	settings    settings.Config
}

func prepareSession(ctx context.Context, cfg Config) (preparedSession, error) {
	if cfg.Runtime == nil {
		return preparedSession{}, errors.New("session: a runtime is required")
	}
	configured := settings.Default()
	if cfg.Settings != nil {
		configured = cfg.Settings.Clone()
	}
	if err := configured.Validate(); err != nil {
		return preparedSession{}, fmt.Errorf("session settings: %w", err)
	}
	keys, err := configuredKeys(configured)
	if err != nil {
		return preparedSession{}, err
	}
	opened, err := session.Open(ctx, cfg.Runtime, cfg.SessionID, cfg.Workspace)
	if err != nil {
		return preparedSession{}, err
	}
	attachments, err := attachment.New(opened.Session.Workspace)
	if err != nil {
		return preparedSession{}, fmt.Errorf("session attachments: %w", err)
	}
	return preparedSession{opened: opened, attachments: attachments, keys: keys, settings: configured}, nil
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
