package main

import (
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/service/tool"
)

// runtime bundles the constructed services. Subcommands receive it
// fully wired from [newRuntime] so they don't repeat the boilerplate.
type runtime struct {
	chat    chat.Service
	session session.Service
	tool    tool.Service
}

// newRuntime loads config + builds the engine + wires services.
// Returns an error suitable for printing to stderr; subcommands
// pass that straight through to [printErr] without further wrapping.
//
// Kept centralised so the day a subcommand grows (or transport
// adapters arrive) the wiring stays in one place.
func newRuntime() (*runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	client, err := config.BuildChatClient(cfg)
	if err != nil {
		return nil, err
	}
	eng, err := engine.New(engine.Config{
		ChatClient: client,
		Online:     config.EngineOnline(cfg),
	})
	if err != nil {
		return nil, err
	}
	return &runtime{
		chat:    chat.New(eng),
		session: session.NewInMemoryService(),
		tool:    tool.New(eng),
	}, nil
}

// printErr writes "lyra: <err>" to stderr and returns a non-zero
// exit code subcommands can return verbatim.
func printErr(err error) int {
	fmt.Fprintf(stderr(), "lyra: %s\n", err)
	return 1
}
