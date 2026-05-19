package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"lyra/internal/agui"
)

// App ties the Wails lifecycle to backing services. The AG-UI mock server is
// the only one for now, but anything else (DB pool, MCP clients, etc.) would
// hang off here too.
type App struct {
	ctx    context.Context
	server *agui.Server
}

func NewApp() *App {
	return &App{
		server: agui.New(""), // empty → DefaultAddr
	}
}

// startup is wired to OnStartup in main.go. Boot anything that needs the app
// context here.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.server.Start(); err != nil {
		log.Printf("agui: failed to start: %v", err)
	}
}

// shutdown is wired to OnShutdown. Best-effort: we give the server two
// seconds to drain before forcing the close so a hung client doesn't hold up
// the app exit.
func (a *App) shutdown(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := a.server.Stop(stopCtx); err != nil {
		log.Printf("agui: shutdown: %v", err)
	}
}

// AGUIURL is exposed to the frontend so the JS side never has to hardcode the
// port — it asks the Go side where to connect.
func (a *App) AGUIURL() string {
	return fmt.Sprintf("http://%s/run", a.server.Addr)
}
