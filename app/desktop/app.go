package main

import "context"

// App ties into the Wails lifecycle. The desktop shell holds NO embedded
// runtime — the frontend talks to the Lyra Runtime over HTTP at its configured
// base URL (frontend `api.baseUrl`, default http://127.0.0.1:17171). Anything
// that needs the app context (e.g. wails runtime calls) boots in startup.
type App struct {
	ctx context.Context
}

func NewApp() *App { return &App{} }

// startup is wired to OnStartup in main.go.
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown is wired to OnShutdown in main.go.
func (a *App) shutdown(_ context.Context) {}
