package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
)

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionConfig] on [Config]; the defaults aim at "comfortably
// fits in 128k-context models" without doing per-turn token math.
const (
	defaultCompactThreshold  = 24 // total messages before we trigger
	defaultCompactKeepRecent = 6  // raw messages to preserve verbatim
)

// CompactionConfig tunes the auto-compaction heuristic.
//
// MaxMessages is the upper bound that triggers a compaction sweep.
// When the conversation grows past it, the oldest
// (len - KeepRecent) messages are replaced by a single system
// message carrying an LLM-generated summary.
//
// Zero values fall back to the package defaults.
type CompactionConfig struct {
	MaxMessages int // default: defaultCompactThreshold
	KeepRecent  int // default: defaultCompactKeepRecent
}

// CompactionResult reports what a single [Engine.MaybeCompact] sweep
// did. Compacted is false (and the counts zero) when the sweep
// didn't fire — no session, history below threshold, or compaction
// disabled. The before/after message counts let the caller surface
// an observable "context compacted (N → M messages)" event instead
// of silently dropping history.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// compactor is the engine's auto-compaction worker. Constructed
// lazily in [Engine.MaybeCompact] (so engines without a memory
// store + chat client both skip silently).
type compactor struct {
	store      memory.Store
	client     *chat.Client
	threshold  int
	keepRecent int
}

func newCompactor(store memory.Store, client *chat.Client, cfg CompactionConfig) *compactor {
	threshold := cfg.MaxMessages
	if threshold <= 0 {
		threshold = defaultCompactThreshold
	}
	keep := cfg.KeepRecent
	if keep <= 0 {
		keep = defaultCompactKeepRecent
	}
	// Sanity: keep must be < threshold or compaction would loop on
	// the same message set.
	if keep >= threshold {
		keep = threshold / 2
	}
	return &compactor{
		store:      store,
		client:     client,
		threshold:  threshold,
		keepRecent: keep,
	}
}

// maybeCompact inspects sessionID's history. When the message
// count exceeds the threshold the older slice is summarized and
// the store is rewritten as [summary, recent...]. The returned
// [CompactionResult] reports whether the sweep fired and the
// before/after message counts so callers can both chain follow-on
// work (e.g. extraction) and surface an observable boundary event.
//
// Important: the summary call goes through chat.Client directly
// (no middleware), so it does NOT enter the chat-memory middleware
// — otherwise the summarisation request itself would be appended
// to the history and trigger another compaction round.
func (c *compactor) maybeCompact(ctx context.Context, sessionID string) (CompactionResult, error) {
	if c == nil || sessionID == "" {
		return CompactionResult{}, nil
	}
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	if len(msgs) < c.threshold {
		return CompactionResult{}, nil
	}

	before := len(msgs)
	cutoff := len(msgs) - c.keepRecent
	older := msgs[:cutoff]
	recent := msgs[cutoff:]

	summary, err := c.summarize(ctx, older)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	if err := c.store.Clear(ctx, sessionID); err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: clear: %w", err)
	}
	rewritten := make([]chat.Message, 0, 1+len(recent))
	rewritten = append(rewritten, summary)
	rewritten = append(rewritten, recent...)
	if err := c.store.Write(ctx, sessionID, rewritten...); err != nil {
		return CompactionResult{}, err
	}
	return CompactionResult{
		Compacted:      true,
		MessagesBefore: before,
		MessagesAfter:  len(rewritten),
	}, nil
}

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *compactor) summarize(ctx context.Context, msgs []chat.Message) (chat.Message, error) {
	transcript := renderTranscript(msgs)
	const prompt = `You are summarizing the earlier portion of a coding-agent
conversation that has grown too long to keep verbatim. Produce a
compact, faithful summary the agent can use to continue without
losing key context.

Format the summary as markdown bullets. Cover:
- Tasks completed
- Files / paths touched
- Open questions or remaining work
- User preferences / constraints stated
- Tool invocations of lasting relevance

Be specific and concise. Quote literal identifiers (file names,
function names) so they remain greppable. Do NOT include the
preamble or the user message; the agent will receive your bullets
verbatim as part of its system prompt.`

	text, err := askDirect(ctx, c.client, prompt, transcript)
	if err != nil {
		return nil, err
	}

	body := "[Earlier conversation summary]\n" + strings.TrimSpace(text)
	return chat.NewSystemMessage(body), nil
}

// renderTranscript flattens messages into a plain-text role-tagged
// transcript the summariser can read. Lossy by design — tool-call
// arguments and parts are flattened to their text bodies; what we
// need is gist, not fidelity.
func renderTranscript(msgs []chat.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		switch m := msg.(type) {
		case *chat.SystemMessage:
			b.WriteString("[system] ")
			b.WriteString(m.Text)
		case *chat.UserMessage:
			b.WriteString("[user] ")
			b.WriteString(m.Text)
		case *chat.AssistantMessage:
			b.WriteString("[assistant] ")
			b.WriteString(m.JoinedText())
		case *chat.ToolMessage:
			b.WriteString("[tool] ")
			for _, r := range m.ToolReturns {
				if r != nil {
					b.WriteString(r.Result)
					b.WriteString(" ")
				}
			}
		default:
			b.WriteString(fmt.Sprintf("[%s] (unrecognized)", msg.Type()))
		}
		b.WriteString("\n")
	}
	return b.String()
}
