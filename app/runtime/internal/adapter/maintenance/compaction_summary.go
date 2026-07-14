package maintenance

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

const compactionPrompt = `You are compacting the earlier portion of a long coding-agent
conversation into a faithful, STRUCTURED summary the agent will read as part of
its system prompt to continue WITHOUT losing key context. Be specific; quote
literal identifiers (file paths, function / type names, commands) so they stay
greppable.

Output markdown under EXACTLY these headings (drop a heading only if truly empty):

## Goal
The user's original objective(s), in their own framing — quote the key request.

## Progress
What has been done so far: completed steps, what worked.

## Current state
Files / paths created or modified (with their paths) + each one's role; key
identifiers (functions, types, symbols) in play; command results worth keeping.

## Decisions & constraints
Choices made and WHY; user preferences / constraints stated (style, libraries,
dos & don'ts); approaches rejected and the reason (so they aren't retried).

## Next steps
Remaining work + open questions — concrete and ordered.

Do NOT echo this instruction or restate the raw transcript; the agent receives
your sections verbatim.`

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *Compactor) summarize(ctx context.Context, msgs []chat.Message) (chat.Message, error) {
	transcript := renderTranscript(msgs, summaryToolResultCap)

	var client *chatclient.Client
	if c.client != nil {
		client = c.client(ctx)
	}
	text, err := askDirect(ctx, client, compactionPrompt, transcript)
	if err != nil {
		return chat.Message{}, err
	}

	body := "[Earlier conversation summary]\n" + strings.TrimSpace(text)
	return chat.NewSystemMessage(body), nil
}
