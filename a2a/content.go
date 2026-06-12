package a2a

import (
	"encoding/json"
	"fmt"
	"strings"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
)

// userMessage builds an A2A user message carrying a single text part — the
// shape a caller sends when delegating to a remote agent.
func userMessage(text string) *sdka2a.Message {
	return sdka2a.NewMessage(sdka2a.MessageRoleUser, sdka2a.NewTextPart(text))
}

// flattenParts renders A2A content parts to a single string: text parts are
// concatenated verbatim, structured data parts are JSON-encoded, and other
// kinds (raw bytes, file URLs) are described compactly. tools and the
// chat loop are text-first, so this is the lossy-but-faithful projection —
// the analog of mcp.flattenContent.
func flattenParts(parts sdka2a.ContentParts) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == nil {
			continue
		}
		switch content := part.Content.(type) {
		case sdka2a.Text:
			b.WriteString(string(content))
		case sdka2a.Data:
			if raw, err := json.Marshal(content.Value); err == nil {
				b.Write(raw)
			} else {
				// Don't let the part vanish silently — leave a marker so the
				// reader knows something was here.
				b.WriteString("[unrenderable data]")
			}
		case sdka2a.URL:
			b.WriteString(string(content))
		case sdka2a.Raw:
			// Binary payloads have no faithful text form; note the size.
			fmt.Fprintf(&b, "[binary content, %d bytes]", len(content))
		}
	}
	return b.String()
}

// resultText extracts the reply text from a SendMessageResult and reports a
// *RemoteAgentError when the remote ended the task in a non-successful
// terminal state. A direct Message reply yields its parts; a Task reply
// prefers its artifacts, falling back to the status message.
func resultText(result sdka2a.SendMessageResult) (string, error) {
	switch r := result.(type) {
	case *sdka2a.Message:
		return flattenParts(r.Parts), nil
	case *sdka2a.Task:
		switch r.Status.State {
		case sdka2a.TaskStateFailed, sdka2a.TaskStateRejected, sdka2a.TaskStateCanceled:
			return "", &RemoteAgentError{State: r.Status.State, Detail: statusDetail(r)}
		}
		return taskText(r), nil
	default:
		return "", nil
	}
}

// taskText concatenates a task's artifact parts, falling back to its status
// message when no artifacts are present.
func taskText(task *sdka2a.Task) string {
	var b strings.Builder
	for _, artifact := range task.Artifacts {
		if artifact != nil {
			b.WriteString(flattenParts(artifact.Parts))
		}
	}
	if b.Len() == 0 {
		return statusDetail(task)
	}
	return b.String()
}

func statusDetail(task *sdka2a.Task) string {
	if task.Status.Message == nil {
		return ""
	}
	return flattenParts(task.Status.Message.Parts)
}
