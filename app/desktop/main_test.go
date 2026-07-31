package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestDesktopApplicationWindowGeometry(t *testing.T) {
	app := desktopApplicationOptions()

	if app.Width != 1440 || app.Height != 900 {
		t.Fatalf("default window size = %dx%d, want 1440x900", app.Width, app.Height)
	}
	if app.MinWidth != 1120 || app.MinHeight != 720 {
		t.Fatalf("minimum window size = %dx%d, want 1120x720", app.MinWidth, app.MinHeight)
	}
	if app.MinWidth > app.Width || app.MinHeight > app.Height {
		t.Fatalf(
			"minimum window size %dx%d exceeds default %dx%d",
			app.MinWidth,
			app.MinHeight,
			app.Width,
			app.Height,
		)
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
