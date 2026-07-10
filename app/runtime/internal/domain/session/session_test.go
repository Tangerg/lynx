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

func TestSessionEffectiveModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		fallback string
		want     string
	}{
		{"explicit model wins", "claude-opus-4-8", "gpt-5", "claude-opus-4-8"},
		{"empty falls back", "", "gpt-5", "gpt-5"},
		{"empty and empty fallback stays empty", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Session{Model: tc.model}
			if got := s.EffectiveModel(tc.fallback); got != tc.want {
				t.Errorf("EffectiveModel(%q) = %q, want %q", tc.fallback, got, tc.want)
			}
		})
	}
}

func TestSessionFork(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	parent := Session{
		ID:       "ses_parent",
		Title:    "research",
		Cwd:      "/work/proj",
		Model:    "claude-opus-4-8",
		Metadata: map[string]any{"k": "v"},
	}

	child := parent.Fork("ses_child", "msg-7", now)

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
	if child.Metadata[ForkAtMessageIDKey] != "msg-7" {
		t.Errorf("Metadata[%s] = %q, want msg-7", ForkAtMessageIDKey, child.Metadata[ForkAtMessageIDKey])
	}
	if !child.StartedAt.Equal(now) || !child.UpdatedAt.Equal(now) {
		t.Errorf("timestamps = %v / %v, want %v", child.StartedAt, child.UpdatedAt, now)
	}
	// A fork starts a fresh conversation: the parent's model is not inherited.
	if child.Model != "" {
		t.Errorf("Model = %q, want empty (not inherited)", child.Model)
	}
}

func TestSessionNewSubtask(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	parent := Session{ID: "ses_parent", Title: "research", Cwd: "/work/proj", Model: "claude-opus-4-8"}

	child := parent.NewSubtask("ses_child", now)

	if child.ID != "ses_child" {
		t.Errorf("ID = %q, want ses_child", child.ID)
	}
	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %q, want %q", child.ParentID, parent.ID)
	}
	if child.Title != "research · subtask" {
		t.Errorf("Title = %q, want %q", child.Title, "research · subtask")
	}
	if child.Cwd != parent.Cwd {
		t.Errorf("Cwd = %q, want inherited %q", child.Cwd, parent.Cwd)
	}
	if child.Kind != KindSubtask {
		t.Errorf("Kind = %q, want %q", child.Kind, KindSubtask)
	}
	if !child.StartedAt.Equal(now) || !child.UpdatedAt.Equal(now) {
		t.Errorf("timestamps = %v / %v, want %v", child.StartedAt, child.UpdatedAt, now)
	}
	if child.Model != "" {
		t.Errorf("subtask started fresh? Model=%q", child.Model)
	}

	// An untitled parent (the id-only stand-in the adapter passes when the
	// parent is missing) yields the bare "subtask" title, no dangling separator.
	if got := (Session{ID: "ses_p"}).NewSubtask("ses_c", now).Title; got != "subtask" {
		t.Errorf("untitled-parent subtask title = %q, want %q", got, "subtask")
	}
}
