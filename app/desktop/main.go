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

func desktopApplicationOptions() *options.App {
	return &options.App{
		Title:     "lyra",
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		MinWidth:  minimumWindowWidth,
		MinHeight: minimumWindowHeight,
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
	}
}

func main() {
	if err := wails.Run(desktopApplicationOptions()); err != nil {
		log.Fatal(err)
	}
}
