package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Tangerg/lynx/app2/desktop/remote"
	"github.com/Tangerg/lynx/app2/desktop/supervisor"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	defaultWindowWidth  = 1440
	defaultWindowHeight = 900
	minimumWindowWidth  = 1120
	minimumWindowHeight = 720
)

func main() {
	if err := runDesktop(); err != nil {
		slog.Error("Desktop stopped", "error", err)
		os.Exit(1)
	}
}

func runDesktop() (err error) {
	config, err := defaultSupervisorConfig()
	if err != nil {
		return err
	}
	runtimeSupervisor, err := supervisor.New(config)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err = errors.Join(err, runtimeSupervisor.Close(ctx))
	}()
	if _, err := runtimeSupervisor.Start(context.Background()); err != nil {
		return fmt.Errorf("desktop: start local Runtime: %w", err)
	}
	remoteManager, err := remote.Open(filepath.Join(config.DataHome, "desktop-remote.json"))
	if err != nil {
		return err
	}
	defer remoteManager.Close()
	desktopHost, err := newDesktopHost(runtimeSupervisor, remoteManager)
	if err != nil {
		return err
	}

	app := application.New(application.Options{
		Name: "lyra", Description: "Agent client for the Lyra Runtime",
		Services: []application.Service{application.NewService(desktopHost)},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:      application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		OnShutdown: func() {
			remoteManager.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			if closeErr := runtimeSupervisor.Close(ctx); closeErr != nil {
				slog.Error("Runtime supervisor shutdown failed", "error", closeErr)
			}
		},
	})
	window := app.Window.NewWithOptions(desktopWindowOptions())
	nativeHost, err := newNativeHost(
		window,
		wailsDirectoryPicker{dialogs: app.Dialog, window: window},
		wailsImageSaver{dialogs: app.Dialog, window: window},
		wailsSessionDocumentTransfer{dialogs: app.Dialog, window: window},
	)
	if err != nil {
		return err
	}
	app.RegisterService(application.NewService(nativeHost))
	return app.Run()
}

func defaultSupervisorConfig() (supervisor.Config, error) {
	userHome := os.Getenv("LYRA2_USER_HOME")
	var err error
	if userHome == "" {
		userHome, err = os.UserHomeDir()
	}
	if err != nil || !filepath.IsAbs(userHome) {
		return supervisor.Config{}, errors.New("desktop: LYRA2_USER_HOME or the system user home must be absolute")
	}
	userHome = filepath.Clean(userHome)
	executable, err := os.Executable()
	if err != nil {
		return supervisor.Config{}, fmt.Errorf("desktop: locate executable: %w", err)
	}
	runtimeBinary := os.Getenv("LYRA2_RUNTIME_BINARY")
	if runtimeBinary == "" {
		runtimeBinary = filepath.Join(filepath.Dir(executable), "lyra-runtime")
	}
	if !filepath.IsAbs(runtimeBinary) {
		return supervisor.Config{}, errors.New("desktop: LYRA2_RUNTIME_BINARY must be absolute")
	}
	return supervisor.Config{
		RuntimeBinary:    filepath.Clean(runtimeBinary),
		DataHome:         filepath.Join(userHome, ".lyra-app2"),
		DefaultWorkspace: userHome,
		UserHome:         userHome,
	}, nil
}

func desktopWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title: "lyra", Width: defaultWindowWidth, Height: defaultWindowHeight,
		MinWidth: minimumWindowWidth, MinHeight: minimumWindowHeight, URL: "/",
		BackgroundColour: application.NewRGB(246, 246, 243),
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true, HideTitle: true, FullSizeContent: true,
				UseToolbar: true, HideToolbarSeparator: true,
				ToolbarStyle: application.MacToolbarStyleUnifiedCompact,
			},
			Appearance: application.NSAppearanceNameAqua,
		},
	}
}
