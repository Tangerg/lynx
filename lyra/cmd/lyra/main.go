// Command lyra is the M1 walking-skeleton entrypoint. It wires the
// loaded config → chat client → engine → ChatService, runs one turn
// with the message supplied on argv, prints the streamed events.
//
// Layer breakdown (see lyra/doc/ARCHITECTURE.md):
//
//	main.go                     // wiring only
//	  └─ config.Load            // env-based config
//	  └─ config.BuildChatClient // pick lynx model adapter
//	  └─ engine.New             // build platform + agent
//	  └─ chat.New               // ChatService implementation
//	  └─ svc.StartTurn          // run one turn
//	  └─ svc.Events             // drain event stream
//
// Transport layer (HTTP/gRPC/IPC) is *not* in M1 — callers use the
// Go ChatService interface directly. Adapters arrive in Phase 2.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lyra:", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()
	message := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if message == "" {
		return fmt.Errorf("usage: lyra <message>")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client, err := config.BuildChatClient(cfg)
	if err != nil {
		return err
	}
	eng, err := engine.New(engine.Config{ChatClient: client})
	if err != nil {
		return err
	}

	svc := chat.New(eng)
	ctx := context.Background()

	handle, err := svc.StartTurn(ctx, chat.StartTurnRequest{
		SessionID: "cli-" + uuid.NewString(),
		Message:   message,
	})
	if err != nil {
		return err
	}

	events, err := svc.Events(ctx, handle)
	if err != nil {
		return err
	}

	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnStart:
			fmt.Fprintf(os.Stderr, "[lyra] turn %s started (model=%s)\n", e.TurnID[:8], e.Model)
		case chat.MessageDelta:
			fmt.Print(e.Text)
		case chat.ToolCallStart:
			fmt.Fprintf(os.Stderr, "\n[lyra] tool start: %s\n", e.ToolName)
		case chat.ToolCallEnd:
			fmt.Fprintf(os.Stderr, "[lyra] tool end (%d bytes)\n", len(e.Output))
		case chat.ErrorEvent:
			fmt.Fprintf(os.Stderr, "\n[lyra] error: %s (%s)\n", e.Message, e.Code)
		case chat.TurnEnd:
			fmt.Fprintf(os.Stderr, "\n[lyra] turn ended: %s (%s)\n", e.Reason, e.Duration)
		}
	}

	return nil
}
