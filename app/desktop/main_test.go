package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDesktopWindowGeometry(t *testing.T) {
	window := desktopWindowOptions()

	if window.Width != 1440 || window.Height != 900 {
		t.Fatalf("default window size = %dx%d, want 1440x900", window.Width, window.Height)
	}
	if window.MinWidth != 1120 || window.MinHeight != 720 {
		t.Fatalf("minimum window size = %dx%d, want 1120x720", window.MinWidth, window.MinHeight)
	}
	if window.MinWidth > window.Width || window.MinHeight > window.Height {
		t.Fatalf(
			"minimum window size %dx%d exceeds default %dx%d",
			window.MinWidth,
			window.MinHeight,
			window.Width,
			window.Height,
		)
	}
}

// The one assertion that replaced a file. The compact toolbar style is what pins the
// titlebar at 40pt, and 40pt is what puts the platform's marks on the center line of the
// app's 42pt header; the automatic style resolves to 66pt and drops them 26pt down, well
// below any header. It used to be set from Objective-C after the window existed, because
// v2 had no option for it — so nothing could assert it. Here it is a value.
func TestDesktopWindowPinsTheCompactToolbarStyle(t *testing.T) {
	titleBar := desktopWindowOptions().Mac.TitleBar

	if titleBar.ToolbarStyle != application.MacToolbarStyleUnifiedCompact {
		t.Fatalf("toolbar style = %v, want unified compact", titleBar.ToolbarStyle)
	}
	if !titleBar.UseToolbar {
		t.Fatal("a toolbar style with no toolbar is not applied; UseToolbar must stay true")
	}
	// Dropping NSWindowStyleMaskTitled takes the frame buttons and the window frame with
	// them — square corners, no shadow. The app draws its header under the platform's
	// title bar rather than replacing it.
	if titleBar.Hide {
		t.Fatal("the title bar is transparent, not hidden")
	}
}

func TestDesktopApplicationBindsHostAsItsOnlyService(t *testing.T) {
	host := mustDesktopHost(t, t.TempDir())
	services := desktopApplicationOptions(host).Services

	want := []application.Service{application.NewService(host)}
	if !reflect.DeepEqual(services, want) {
		t.Fatalf("services = %#v, want exactly the configured host", services)
	}
}

func TestDesktopMinimumGeometryMatchesFrontendShell(t *testing.T) {
	css, err := os.ReadFile("frontend/src/styles/globals.css")
	if err != nil {
		t.Fatalf("read frontend shell geometry: %v", err)
	}

	for property, value := range map[string]int{
		"--app-min-width":  minimumWindowWidth,
		"--app-min-height": minimumWindowHeight,
	} {
		declaration := fmt.Sprintf("%s: %dpx;", property, value)
		if !strings.Contains(string(css), declaration) {
			t.Errorf("frontend shell is missing %q", declaration)
		}
	}
}
