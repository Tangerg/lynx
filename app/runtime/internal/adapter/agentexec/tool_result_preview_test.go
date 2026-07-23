package agentexec

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderToolResultPreviewIncludesBoundedHeadTailAndRetrievalIdentity(t *testing.T) {
	body := strings.Repeat("a", 250) + strings.Repeat("z", 250)
	preview := renderToolResultPreview(body, "ABC234XYZ7", "read_tool_result", 100)

	if !strings.HasPrefix(preview, strings.Repeat("a", 75)) {
		t.Fatal("preview does not preserve the configured head")
	}
	if !strings.HasSuffix(preview, strings.Repeat("z", 25)) {
		t.Fatal("preview does not preserve the configured tail")
	}
	if !strings.Contains(preview, `read_tool_result tool: {"id":"ABC234XYZ7"}`) {
		t.Fatal("preview does not carry the retrieval affordance")
	}
}

func TestRenderToolResultPreviewPreservesUTF8(t *testing.T) {
	body := strings.Repeat("界", 200)
	preview := renderToolResultPreview(body, "ABCDE234", "read_tool_result", 100)
	if !utf8.ValidString(preview) {
		t.Fatal("preview split a multibyte rune")
	}
}
