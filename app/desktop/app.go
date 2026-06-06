package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"lyra/internal/mockruntime"
)

// App ties the Wails lifecycle to backing services. The AG-UI mock server is
// the only one for now, but anything else (DB pool, MCP clients, etc.) would
// hang off here too.
type App struct {
	ctx    context.Context
	server *mockruntime.Server
}

func NewApp() *App {
	return &App{
		server: mockruntime.New(""), // empty → DefaultAddr
	}
}

// startup is wired to OnStartup in main.go. Boot anything that needs the app
// context here.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Mock is OFF by default now that we integrate the real lynx Runtime —
	// this keeps :17171 free for it. Set LYRA_MOCK=1 to bring the demo mock
	// back (e.g. `LYRA_MOCK=1 wails dev`).
	if !mockEnabled() {
		log.Printf("mockruntime: embedded mock disabled (set LYRA_MOCK=1 to enable); " +
			"frontend will talk to the runtime on its configured base URL")
		return
	}
	if err := a.server.Start(); err != nil {
		log.Printf("mockruntime: failed to start: %v", err)
	}
}

// mockEnabled reports whether the embedded AG-UI demo mock should bind.
func mockEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LYRA_MOCK"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// shutdown is wired to OnShutdown. Best-effort: we give the server two
// seconds to drain before forcing the close so a hung client doesn't hold up
// the app exit.
func (a *App) shutdown(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := a.server.Stop(stopCtx); err != nil {
		log.Printf("mockruntime: shutdown: %v", err)
	}
}

// AGUIURL is exposed to the frontend so the JS side never has to hardcode the
// port — it asks the Go side where to connect.
func (a *App) AGUIURL() string {
	return fmt.Sprintf("http://%s/run", a.server.Addr)
}
