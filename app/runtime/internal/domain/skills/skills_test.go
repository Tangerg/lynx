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

func TestDraftHandleBindsNameAndContent(t *testing.T) {
	content := []byte("approved bytes")
	handle := NewDraftHandle("approved-skill", content)
	if err := handle.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !handle.Matches(content) {
		t.Fatal("handle does not match its source content")
	}
	if handle.Matches([]byte("different bytes")) {
		t.Fatal("handle matched different content")
	}
	if (DraftHandle{Name: "other", Revision: handle.Revision}).Matches(content) {
		t.Fatal("handle revision was accepted for another skill name")
	}
}

func TestDraftHandleRejectsMalformedRevision(t *testing.T) {
	for _, handle := range []DraftHandle{
		{},
		{Name: "Bad Name", Revision: NewDraftHandle("Bad Name", nil).Revision},
		{Name: "skill", Revision: "short"},
		{Name: "skill", Revision: "zz" + NewDraftHandle("skill", nil).Revision[2:]},
	} {
		if err := handle.Validate(); err == nil {
			t.Fatalf("Validate(%+v) = nil, want error", handle)
		}
	}
}
