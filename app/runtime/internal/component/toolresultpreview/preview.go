// Package toolresultpreview renders the model-facing preview for a tool result
// whose full body lives in durable blob storage. Durable consumers use a typed
// offload.Ref; they never recover identity by parsing this presentation text.
package toolresultpreview

import (
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
)

// Render returns a head-and-tail inline replacement that tells the model how
// to retrieve the full result. Cuts snap to rune boundaries. Callers decide
// whether the rendered value is materially smaller before using it because the
// retrieval marker itself has a fixed cost.
func Render(body string, id offload.ID, readToolName string, previewBytes int) string {
	previewBytes = min(max(previewBytes, 0), len(body))
	head := previewBytes * 3 / 4
	tailStart := len(body) - previewBytes/4
	for head > 0 && !utf8.RuneStart(body[head]) {
		head--
	}
	for tailStart < len(body) && !utf8.RuneStart(body[tailStart]) {
		tailStart++
	}
	if tailStart < head {
		tailStart = head
	}
	marker := fmt.Sprintf(
		"\n\n…[%d bytes offloaded to keep the context small — retrieve the full output with the %s tool: {\"id\":\"%s\"} (supports offset/limit).]…\n\n",
		len(body), readToolName, id,
	)
	return body[:head] + marker + body[tailStart:]
}
