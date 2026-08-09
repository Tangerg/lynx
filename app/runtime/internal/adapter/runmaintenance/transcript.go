package runmaintenance

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/core/chat"
)

// uncappedToolResults is the [renderTranscript] toolResultCap that leaves tool
// bodies intact — used by the trigger estimate and the fact consolidator, which
// must see the real footprint / full content (only the summariser caps).
const uncappedToolResults = 0

// renderTranscript flattens messages into a plain-text role-tagged
// transcript a summariser / consolidator can read. Lossy by design — tool-call
// arguments and parts are flattened to their text bodies; what we
// need is gist, not fidelity.
//
// toolResultCap > 0 truncates each tool-result body to that many bytes (head +
// tail, with the elision marked); 0 leaves bodies intact. The summariser passes
// a cap so a few large tool outputs (the very thing the token trigger fires on)
// don't dominate its own input; the trigger estimate and the fact consolidator
// pass 0 because they must see the real footprint / full content.
func renderTranscript(msgs []chat.Message, toolResultCap int) string {
	var b strings.Builder
	for _, msg := range msgs {
		switch msg.Role {
		case chat.RoleSystem:
			b.WriteString("[system] ")
			b.WriteString(msg.Text())
		case chat.RoleUser:
			b.WriteString("[user] ")
			b.WriteString(msg.Text())
		case chat.RoleAssistant:
			b.WriteString("[assistant] ")
			b.WriteString(msg.Text())
		case chat.RoleTool:
			b.WriteString("[tool] ")
			for _, part := range msg.Parts {
				if part.Kind == chat.PartToolResult && part.ToolResult != nil {
					b.WriteString(capText(part.ToolResult.Result, toolResultCap))
					b.WriteString(" ")
				}
			}
		default:
			fmt.Fprintf(&b, "[%s] (unrecognized)", msg.Role)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// headTail reduces s to a head (¾) + tail (¼) preview when it exceeds limit,
// the elided middle replaced by marker(elidedChars). Cuts snap to rune
// boundaries so a multibyte rune is never split. Returns s unchanged when
// limit <= 0 or s already fits. Shared by the summariser's input cap
// ([capText]) and the compaction ladder's stored trim (compaction_ladder.go);
// each supplies its own marker.
func headTail(s string, limit int, marker func(elided int) string) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	head, tailStart := limit*3/4, len(s)-limit/4
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	return s[:head] + marker(tailStart-head) + s[tailStart:]
}

// capText bounds an oversized body for the SUMMARISER'S INPUT (transient, not
// stored) — head+tail with the elided middle marked. limit <= 0 or an
// already-small body is returned unchanged.
func capText(s string, limit int) string {
	return headTail(s, limit, func(elided int) string {
		return fmt.Sprintf("\n…[%d bytes elided for summary]…\n", elided)
	})
}
