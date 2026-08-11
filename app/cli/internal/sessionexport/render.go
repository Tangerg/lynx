package sessionexport

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func renderMarkdown(snapshot agent.SessionSnapshot) string {
	var out strings.Builder
	writeLine(&out, "# "+singleLine(snapshot.Session.Title, "Untitled session"))
	writeLine(&out, "")
	writeLine(&out, "- Session: "+inlineCode(snapshot.Session.ID))
	writeLine(&out, "- Workspace: "+inlineCode(snapshot.Session.Workspace))
	writeLine(&out, "- Model: "+inlineCode(singleLine(snapshot.Session.Model, "not selected")))
	writeLine(&out, "- Status: "+inlineCode(string(snapshot.Session.Status)))
	writeLine(&out, "- Updated: "+inlineCode(snapshot.Session.UpdatedAt.Format(time.RFC3339)))
	writeLine(&out, "")
	writeLine(&out, "## Conversation")
	for _, block := range snapshot.Transcript {
		writeMarkdownBlock(&out, block)
	}
	if len(snapshot.Transcript) == 0 {
		writeLine(&out, "")
		writeLine(&out, "_No transcript items._")
	}
	if len(snapshot.Plan) > 0 {
		writeLine(&out, "")
		writeLine(&out, "## Plan")
		writeLine(&out, "")
		for _, item := range snapshot.Plan {
			writeLine(&out, fmt.Sprintf("- [%s] %s", planMark(item.Status), strings.TrimSpace(item.Title)))
		}
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func writeMarkdownBlock(out *strings.Builder, block agent.Block) {
	writeLine(out, "")
	switch block.Kind {
	case agent.BlockUser:
		writeLine(out, "### You")
		writeText(out, block.Text)
		for _, attachment := range block.Attachments {
			writeLine(out, fmt.Sprintf("- Attachment: %s (%s, %d bytes)", inlineCode(attachment.Name), inlineCode(attachment.MimeType), attachment.Size))
		}
	case agent.BlockAssistant:
		writeLine(out, "### Lyra")
		writeText(out, block.Text)
	case agent.BlockReasoning:
		writeLine(out, "### Thinking")
		writeText(out, block.Text)
	case agent.BlockTool:
		writeMarkdownTool(out, block)
	case agent.BlockQuestion:
		writeMarkdownQuestion(out, block)
	case agent.BlockNotice:
		writeLine(out, "### Notice")
		writeText(out, block.Text)
	case agent.BlockError:
		writeLine(out, "### Runtime error")
		writeText(out, block.Text)
	}
}

func writeMarkdownTool(out *strings.Builder, block agent.Block) {
	if block.Tool == nil {
		writeLine(out, "### Tool")
		writeLine(out, "_Tool projection unavailable._")
		return
	}
	tool := *block.Tool
	label := firstNonEmpty(tool.Summary, tool.Command, tool.Path, tool.Query, tool.URL, tool.Name, string(tool.Kind))
	writeLine(out, "### Tool · "+singleLine(label, "unknown"))
	writeLine(out, "")
	writeLine(out, "- Kind: "+inlineCode(string(tool.Kind)))
	writeLine(out, "- Status: "+inlineCode(string(tool.Status)))
	if tool.Duration > 0 {
		writeLine(out, "- Duration: "+inlineCode(tool.Duration.String()))
	}
	if tool.ExitCode != nil {
		writeLine(out, "- Exit code: "+inlineCode(strconv.Itoa(*tool.ExitCode)))
	}
	for _, field := range []struct{ label, value string }{
		{"Command", tool.Command}, {"Path", tool.Path}, {"Query", tool.Query}, {"URL", tool.URL},
	} {
		if strings.TrimSpace(field.value) != "" {
			writeLine(out, fmt.Sprintf("- %s: %s", field.label, inlineCode(strings.TrimSpace(field.value))))
		}
	}
	if strings.TrimSpace(tool.Output) != "" {
		writeLine(out, "")
		writeLine(out, "#### Output")
		writeCode(out, "text", tool.Output)
	}
	if strings.TrimSpace(tool.Diff) != "" {
		writeLine(out, "")
		writeLine(out, "#### Diff")
		writeCode(out, "diff", tool.Diff)
	}
}

func writeMarkdownQuestion(out *strings.Builder, block agent.Block) {
	if block.Question == nil {
		writeLine(out, "### Question")
		writeLine(out, "_Question projection unavailable._")
		return
	}
	question := block.Question
	writeLine(out, "### Question · "+singleLine(question.Title, "Untitled"))
	writeText(out, question.Detail)
	for _, field := range question.Fields {
		line := "- " + strings.TrimSpace(field.Prompt) + " (" + inlineCode(string(field.Kind)) + ")"
		if len(field.Options) > 0 {
			labels := make([]string, 0, len(field.Options))
			for _, option := range field.Options {
				labels = append(labels, option.Label)
			}
			line += ": " + strings.Join(labels, ", ")
		}
		writeLine(out, line)
	}
}

func writeText(out *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	writeLine(out, "")
	writeLine(out, value)
}

func writeCode(out *strings.Builder, language, value string) {
	value = strings.TrimRight(value, "\n")
	fence := strings.Repeat("`", max(3, longestBacktickRun(value)+1))
	writeLine(out, "")
	writeLine(out, fence+language)
	writeLine(out, value)
	writeLine(out, fence)
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		if r == '`' {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	return longest
}

func inlineCode(value string) string {
	value = strings.TrimSpace(value)
	fence := strings.Repeat("`", max(1, longestBacktickRun(value)+1))
	padding := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		padding = " "
	}
	return fence + padding + value + padding + fence
}

func writeLine(out *strings.Builder, value string) {
	out.WriteString(value)
	out.WriteByte('\n')
}

func singleLine(value, fallback string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func planMark(status agent.PlanStatus) string {
	switch status {
	case agent.PlanDone:
		return "x"
	case agent.PlanActive:
		return "~"
	default:
		return " "
	}
}
