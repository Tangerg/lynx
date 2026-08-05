package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	defaultWindowWidth  = 1440
	defaultWindowHeight = 900
	minimumWindowWidth  = 1120
	minimumWindowHeight = 720
)

func desktopApplicationOptions(host *DesktopHost) *options.App {
	return &options.App{
		Title:     "lyra",
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		MinWidth:  minimumWindowWidth,
		MinHeight: minimumWindowHeight,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []any{host},
		// Pinned at startup, where the window first exists. This used to be a
		// DesktopHost method that also stored the context as "the window's
		// identity" — nothing ever read it, because the native chrome is reached
		// through the platform's key window and not through this context. So it
		// belongs beside the options it completes, not on the frontend binding.
		OnStartup:        func(context.Context) { useCompactWindowToolbar() },
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		// macOS: a real titled window whose title bar is transparent and empty,
		// with the content running the full height underneath it. The platform
		// draws its own three controls over that content and the app reserves a
		// gutter for them from the geometry it measures (DesktopHost.WindowChrome);
		// the window moves through the draggable chrome bars.
		//
		// UseToolbar is here for its height, not for a toolbar: it is what makes
		// the titlebar taller than 32pt, and the frame buttons are centred in
		// whatever height it has. `useCompactWindowToolbar` then pins the style,
		// because the automatic one resolves to a titlebar two thirds taller again.
		//
		// HideTitleBar stays false deliberately. Setting it drops
		// NSWindowStyleMaskTitled, which removes the buttons — and the window frame
		// with them, leaving square corners and no shadow.
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				FullSizeContent:            true,
				UseToolbar:                 true,
				HideToolbarSeparator:       true,
			},
			Appearance: mac.NSAppearanceNameAqua,
		},
	}
}

func main() {
	host, err := defaultDesktopHost()
	if err != nil {
		log.Fatal(err)
	}
	if err := wails.Run(desktopApplicationOptions(host)); err != nil {
		log.Fatal(err)
	}
}
