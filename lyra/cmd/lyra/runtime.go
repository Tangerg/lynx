package main

import (
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

// runtime bundles the constructed services. Subcommands receive it
// fully wired from [newRuntime] so they don't repeat the boilerplate.
type runtime struct {
	chat    chat.Service
	session session.Service
	tool    tool.Service
	memory  memory.Service
}

// newRuntime loads config + builds the engine + wires services,
// using on-disk persistence for sessions, chat-memory, and LYRA.md
// memory so all state survives process restart.
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

	sessionSvc, err := storage.NewFileSessionService()
	if err != nil {
		return nil, fmt.Errorf("session storage: %w", err)
	}
	msgStore, err := storage.NewFileMessageStore()
	if err != nil {
		return nil, fmt.Errorf("message storage: %w", err)
	}
	memSvc, err := storage.NewFileMemoryService()
	if err != nil {
		return nil, fmt.Errorf("memory storage: %w", err)
	}

	eng, err := engine.New(engine.Config{
		ChatClient:    client,
		Online:        config.EngineOnline(cfg),
		MemoryStore:   msgStore,
		MemoryService: memSvc,
	})
	if err != nil {
		return nil, err
	}
	return &runtime{
		chat:    chat.New(eng),
		session: sessionSvc,
		tool:    tool.New(eng),
		memory:  memSvc,
	}, nil
}

// Compile-time assertion that the storage type satisfies the
// memory.Service contract the engine consumes — keeps the
// implements relationship honest if either side changes.
var _ memory.Service = (*storage.FileMemoryService)(nil)

// printErr writes "lyra: <err>" to stderr and returns a non-zero
// exit code subcommands can return verbatim.
func printErr(err error) int {
	fmt.Fprintf(stderr(), "lyra: %s\n", err)
	return 1
}
