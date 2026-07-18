// Package offload is the shared vocabulary for context eviction's placeholder:
// when an oversized tool result is offloaded to the blob store, its inline form
// becomes a head+tail preview carrying the blob id. The engine writes that
// placeholder ([Placeholder]) into the conversation + transcript; the delivery
// presenter reads the id back ([ID]) to rehydrate the full body for display.
// Keeping both sides here means the marker format and its parser can never
// drift.
package offload

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// Placeholder renders the inline replacement for an offloaded body: a head
// (¾ previewBytes) + tail (¼ previewBytes) preview around a marker that names
// the blob id and how to retrieve it via readToolName. Cuts snap to rune
// boundaries so a multibyte rune is never split. previewBytes should be smaller
// than the body (callers cap it to the eviction threshold), so the placeholder
// is always smaller than the body that tripped eviction.
func Placeholder(body, id, readToolName string, previewBytes int) string {
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

// idPattern extracts the blob id from a [Placeholder] marker. It anchors on the
// distinctive "bytes offloaded … {"id":"…"}" phrasing and a base32 id
// ([crypto/rand.Text] output) so an ordinary tool result that merely contains a
// JSON id object won't match; a false match degrades harmlessly anyway (the blob
// fetch misses and the original text is shown).
var idPattern = regexp.MustCompile(`bytes offloaded[^\n]*\{"id":"([A-Z2-7]{2,64})"\}`)

// ID reports the blob id embedded in a placeholder produced by [Placeholder],
// or ok=false when s is not such a placeholder. The presenter uses it to decide
// whether a stored tool result needs rehydrating from the blob store.
func ID(s string) (id string, ok bool) {
	m := idPattern.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}
