package fs

import (
	"os"
	"path/filepath"
	"testing"
)

// expandHome turns the ~ the model habitually emits into the home dir, so a
// read/glob/edit on "~/x" resolves to the real file instead of ".../~/x".
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	cases := map[string]string{
		"~":           home,
		"~/":          home, // Join(home, "") cleans to home
		"~/Desktop/x": filepath.Join(home, "Desktop", "x"),
		"relative/x":  "relative/x", // not anchored here — resolve()/rootDir() do that
		"/abs/x":      "/abs/x",
		"~user/x":     "~user/x", // only the current-user form expands
		"a~b":         "a~b",     // ~ not at the start
		"":            "",
	}
	for in, want := range cases {
		if got := expandHome(in); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
