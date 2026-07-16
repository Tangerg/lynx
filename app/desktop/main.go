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

func main() {
	if err := wails.Run(&options.App{
		Title:     "lyra",
		Width:     1440,
		Height:    900,
		MinWidth:  1280,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		// macOS: hide the native titlebar but keep the native traffic-light
		// controls (inset over our content) — these are the ONLY window controls;
		// the app draws none of its own. Window stays draggable from the top
		// region. Light appearance matches the light-first UI default.
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameAqua,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
