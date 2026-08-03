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
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		// macOS: a real titled window whose title bar is transparent and empty,
		// with the content running the full height underneath it. The app draws
		// its own three controls in the gutter it reserves for them and moves
		// the window through the draggable chrome bars; hideNativeWindowButtons
		// takes the platform's controls off the frame at startup.
		//
		// HideTitleBar stays false deliberately. Setting it drops
		// NSWindowStyleMaskTitled, which does remove the buttons — and the
		// window frame with them, leaving square corners and no shadow.
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
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
