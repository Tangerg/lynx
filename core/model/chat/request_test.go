package chat_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewOptions_RequiresModelID(t *testing.T) {
	if _, err := chat.NewOptions(""); err == nil {
		t.Fatal("NewOptions(\"\") must return an error")
	} else if !strings.Contains(err.Error(), "model id") {
		t.Fatalf("error message must mention model id, got %q", err.Error())
	}

	opts, err := chat.NewOptions("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Model != "gpt-4o" {
		t.Fatalf("Model = %q, want gpt-4o", opts.Model)
	}
}

func TestOptions_GetSetExtra(t *testing.T) {
	opts, _ := chat.NewOptions("m")

	// Reading a missing key returns ok=false; the lazy init must not panic.
	if v, ok := opts.Get("missing"); ok || v != nil {
		t.Fatalf("Get(missing) = (%v,%v), want (nil,false)", v, ok)
	}

	opts.Set("response_format", "json_object")
	if v, ok := opts.Get("response_format"); !ok || v != "json_object" {
		t.Fatalf("Get(response_format) = (%v,%v), want (json_object,true)", v, ok)
	}
}

func TestOptions_Clone_DeepCopiesPointersAndSlices(t *testing.T) {
	temp := 0.7
	opts := &chat.Options{
		Model:       "m",
		Temperature: &temp,
		Stop:        []string{"###"},
		Extra:       map[string]any{"a": 1},
	}

	clone := opts.Clone()

	// Mutating the clone must not affect the original.
	*clone.Temperature = 0.1
	clone.Stop[0] = "MUTATED"
	clone.Extra["a"] = 999

	if *opts.Temperature != 0.7 {
		t.Fatalf("Temperature leaked: %v", *opts.Temperature)
	}
	if opts.Stop[0] != "###" {
		t.Fatalf("Stop slice leaked: %q", opts.Stop[0])
	}
	if opts.Extra["a"].(int) != 1 {
		t.Fatalf("Extra map leaked: %v", opts.Extra["a"])
	}
}

func TestOptions_Clone_NilReceiver(t *testing.T) {
	var opts *chat.Options
	if got := opts.Clone(); got != nil {
		t.Fatalf("nil receiver Clone = %v, want nil", got)
	}
}

func TestMergeOptions_RejectsNilBase(t *testing.T) {
	if _, err := chat.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions(nil) must return an error")
	}
}

func TestMergeOptions_OverridesScalarsAndStop(t *testing.T) {
	t1 := 0.3
	t2 := 0.9

	base := &chat.Options{Model: "base", Temperature: &t1, Stop: []string{"a"}}
	override := &chat.Options{Model: "override", Temperature: &t2, Stop: []string{"b", "c"}}

	merged, err := chat.MergeOptions(base, override)
	if err != nil {
		t.Fatalf("MergeOptions err: %v", err)
	}
	if merged.Model != "override" {
		t.Fatalf("Model = %q, want override (last writer wins)", merged.Model)
	}
	if *merged.Temperature != 0.9 {
		t.Fatalf("Temperature = %v, want 0.9", *merged.Temperature)
	}
	// Stop is replaced, not appended — consistent with every other
	// scalar field overriding on non-zero, and the only way to keep
	// MergeOptions idempotent (see TestMergeOptions_StopIsIdempotent).
	if got := strings.Join(merged.Stop, ","); got != "b,c" {
		t.Fatalf("Stop = %q, want b,c (override replaces)", got)
	}
}

// TestMergeOptions_StopIsIdempotent pins the contract that applying the
// same override repeatedly never multiplies stop sequences. Catches the
// historical append-instead-of-replace bug.
func TestMergeOptions_StopIsIdempotent(t *testing.T) {
	base := &chat.Options{Stop: []string{"a"}}
	override := &chat.Options{Stop: []string{"b"}}

	once, _ := chat.MergeOptions(base, override)
	thrice, _ := chat.MergeOptions(base, override, override, override)

	if strings.Join(once.Stop, ",") != strings.Join(thrice.Stop, ",") {
		t.Fatalf("merge not idempotent: once=%v thrice=%v", once.Stop, thrice.Stop)
	}
}

func TestMergeOptions_KeepsZeroOverrides(t *testing.T) {
	base := &chat.Options{Model: "base"}
	merged, err := chat.MergeOptions(base, &chat.Options{Model: ""})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "base" {
		t.Fatalf("empty override should not overwrite, got %q", merged.Model)
	}
}

func TestMergeOptions_NilOverridesAreSkipped(t *testing.T) {
	base := &chat.Options{Model: "base"}
	merged, err := chat.MergeOptions(base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "base" {
		t.Fatalf("Model = %q, want base", merged.Model)
	}
}

func TestNewRequest_RejectsAllNilMessages(t *testing.T) {
	if _, err := chat.NewRequest(nil); err == nil {
		t.Fatal("NewRequest(nil) must return an error")
	}
	if _, err := chat.NewRequest([]chat.Message{nil, nil}); err == nil {
		t.Fatal("NewRequest with all-nil entries must return an error")
	}
}

func TestNewRequest_FiltersNilEntries(t *testing.T) {
	user := chat.NewUserMessage("hi")
	req, err := chat.NewRequest([]chat.Message{nil, user, nil})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(req.Messages); got != 1 {
		t.Fatalf("len(Messages) = %d, want 1 (nil entries should be dropped)", got)
	}
}

func TestRequest_GetSetParamsLazyInit(t *testing.T) {
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	// Lazy init: zero-value Params must not panic on Get.
	req.Params = nil
	if v, ok := req.Get("missing"); ok || v != nil {
		t.Fatalf("Get(missing) = (%v,%v), want (nil,false)", v, ok)
	}
	req.Set("trace_id", "abc-123")
	if v, _ := req.Get("trace_id"); v != "abc-123" {
		t.Fatalf("Get(trace_id) = %v, want abc-123", v)
	}
}

func TestRequest_AppendToLastUserMessage(t *testing.T) {
	user := chat.NewUserMessage("first")
	req, _ := chat.NewRequest([]chat.Message{user})

	req.AppendToLastUserMessage("second")

	if got := user.Text; !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("appended text not present: %q", got)
	}
}

func TestRequest_ReplaceOfLastUserMessage(t *testing.T) {
	user := chat.NewUserMessage("original")
	req, _ := chat.NewRequest([]chat.Message{user})

	req.ReplaceOfLastUserMessage("replaced")

	if user.Text != "replaced" {
		t.Fatalf("Text = %q, want replaced", user.Text)
	}
}

func TestRequest_UserMessage_ReturnsEmptyWhenAbsent(t *testing.T) {
	req, _ := chat.NewRequest([]chat.Message{chat.NewSystemMessage("be brief")})

	got := req.UserMessage()
	if got == nil {
		t.Fatal("UserMessage must not return nil; it returns a fresh empty *UserMessage")
	}
	if got.Text != "" {
		t.Fatalf("empty UserMessage Text = %q, want empty", got.Text)
	}
}

func TestRequest_SystemMessage_ReturnsEmptyWhenAbsent(t *testing.T) {
	req, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})

	got := req.SystemMessage()
	if got == nil {
		t.Fatal("SystemMessage must not return nil; it returns a fresh empty *SystemMessage")
	}
}

// mustNewTool builds a Tool with a unique name for use across tests.
func mustNewTool(t *testing.T, name string) chat.Tool {
	t.Helper()
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: name, InputSchema: `{"type":"object"}`},
		chat.ToolMetadata{},
		func(context.Context, string) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	return tool
}
