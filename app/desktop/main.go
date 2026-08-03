package main

import (
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
		Bind:             []any{host},
		OnStartup:        host.attachWindow,
		BackgroundColour: &options.RGBA{R: 251, G: 251, B: 252, A: 1},
		// macOS: no title bar at all, so the system draws no window buttons.
		// The app draws its own three controls in the gutter it already reserves
		// for them, and moves the window through the draggable chrome bars.
		//
		// Dropping NSWindowStyleMaskTitled is what removes the buttons. The
		// alternative — keeping them and positioning our chrome around them — is
		// what we had, and it never lined up: their box is the system's, it
		// changes with the appearance and the toolbar, and nothing in the window
		// can measure it.
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               true,
				FullSizeContent:            true,
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
