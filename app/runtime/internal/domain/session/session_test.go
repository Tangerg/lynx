package session

import (
	"errors"
	"testing"
	"time"
)

func TestPatchNormalize(t *testing.T) {
	tests := []struct {
		name    string
		title   *string
		want    string
		wantErr error
	}{
		{name: "absent title"},
		{name: "trims title", title: stringPointer("  renamed  "), want: "renamed"},
		{name: "rejects blank title", title: stringPointer("  "), wantErr: ErrTitleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (Patch{Title: tt.title}).Normalize()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Normalize() error = %v, want %v", err, tt.wantErr)
			}
			if tt.title == nil {
				if got.Title != nil {
					t.Fatalf("Title = %q, want nil", *got.Title)
				}
				return
			}
			if err == nil && (got.Title == nil || *got.Title != tt.want) {
				t.Fatalf("Title = %v, want %q", got.Title, tt.want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestSessionFork(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	parent := Session{
		ID:    "ses_parent",
		Title: "research",
		Cwd:   "/work/proj",
		Model: "claude-opus-4-8",
	}

	child := parent.Fork("ses_child", now)

	if child.ID != "ses_child" {
		t.Errorf("ID = %q, want ses_child", child.ID)
	}
	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Title != "research (fork)" {
		t.Errorf("Title = %q, want %q", child.Title, "research (fork)")
	}
	if child.Cwd != parent.Cwd {
		t.Errorf("Cwd = %q, want inherited %q", child.Cwd, parent.Cwd)
	}
	if !child.StartedAt.Equal(now) || !child.UpdatedAt.Equal(now) {
		t.Errorf("timestamps = %v / %v, want %v", child.StartedAt, child.UpdatedAt, now)
	}
	// A fork starts a fresh conversation: the parent's model is not inherited.
	if child.Model != "" {
		t.Errorf("Model = %q, want empty (not inherited)", child.Model)
	}
}
