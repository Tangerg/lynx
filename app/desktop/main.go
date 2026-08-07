package main

import (
	"embed"
	"log"

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

// The application, which owns the process: its name, what it serves, and what the
// frontend may call. Geometry and window chrome are NOT here — in v3 a window is a
// separate object with its own options, which is the honest shape: an application can
// hold several windows, and none of their sizes is a property of the process.
func desktopApplicationOptions(host *DesktopHost) application.Options {
	return application.Options{
		Name:        "lyra",
		Description: "Agent client for the Lyra Runtime",
		// The Wails-owned boundary, and the whole of it: one service, whose methods are
		// the only Go the frontend can reach.
		Services: []application.Service{application.NewService(host)},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// One window, and closing it means quitting. Without this the process outlives
			// its own window and the dock icon stays lit with nothing behind it.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	}
}

// The window: a real titled window whose title bar is transparent and empty, with the
// content running the full height underneath it. The platform draws its own three
// controls over that content and the app reserves a gutter for them from the geometry it
// measures (DesktopHost.WindowChrome); the window moves through the draggable chrome
// bars.
//
// UseToolbar is here for its HEIGHT, not for a toolbar: it is what makes the titlebar
// taller than 32pt, and the frame buttons are centred in whatever height it has. The
// compact style then pins that height at 40pt, which puts the marks 20pt down — within a
// pixel of the centre line of a 42pt header. That last pixel is why the marks' centre is
// measured rather than assumed: the control beside them centres on THEM and the header's
// text on the header, and at the 5pt apart that a toolbarless window leaves them, no
// amount of measuring makes those two read as one row.
//
// An empty toolbar was verified not to take the clicks in that band: hit-testing the
// frame view 8, 16, 24, 36 and 44pt below the window top returns the content view in all
// three toolbar styles, so the header's own controls keep working under it.
//
// `Hide` stays false deliberately. Setting it drops NSWindowStyleMaskTitled, which
// removes the buttons — and the window frame with them, leaving square corners and no
// shadow.
func desktopWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            "lyra",
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minimumWindowWidth,
		MinHeight:        minimumWindowHeight,
		URL:              "/",
		BackgroundColour: application.NewRGB(255, 255, 255),
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBar{
				AppearsTransparent:   true,
				HideTitle:            true,
				FullSizeContent:      true,
				UseToolbar:           true,
				HideToolbarSeparator: true,
				// The reason an Objective-C file used to exist here. v2 owned whether there
				// was a toolbar but not its style, and the automatic style resolves, on a
				// transparent-titlebar window, to a 66pt titlebar with the marks 26pt down —
				// far below where a tool window's header sits. v3 owns the style, so the app
				// declares it rather than reaching into AppKit to set it after the fact.
				ToolbarStyle: application.MacToolbarStyleUnifiedCompact,
			},
			Appearance: application.NSAppearanceNameAqua,
		},
	}
}

func main() {
	host, err := defaultDesktopHost()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(desktopApplicationOptions(host))
	app.Window.NewWithOptions(desktopWindowOptions())
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
