package skills

import (
	"path/filepath"
	"testing"
)

func TestProjectDir(t *testing.T) {
	tests := []struct {
		name    string
		workdir string
		want    string
	}{
		{name: "empty workdir disables project skills"},
		{name: "project workdir", workdir: "/repo", want: filepath.Join("/repo", ProjectSubdir)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectDir(tt.workdir); got != tt.want {
				t.Fatalf("ProjectDir(%q) = %q, want %q", tt.workdir, got, tt.want)
			}
		})
	}
}
