package safeguard

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
)

func (m *safeguardMiddleware) scanOutput(ctx context.Context, resp *chat.Response) (string, bool) {
	if !m.opts.Scope.inspectsOutput() {
		return "", false
	}
	text := resp.TextDelta()
	if text == "" {
		return "", false
	}
	return m.matcher.Match(ctx, text)
}

// scanInputs walks system / user messages and runs each non-empty
// text through the matcher. ToolMessages and AssistantMessages from
// prior turns are skipped — they're not user-authored.
func (m *safeguardMiddleware) scanInputs(ctx context.Context, req *chat.Request) (string, bool) {
	if req == nil {
		return "", false
	}
	for _, msg := range req.Messages {
		var text string
		switch v := msg.(type) {
		case *chat.UserMessage:
			text = v.Text
		case *chat.SystemMessage:
			text = v.Text
		default:
			continue
		}
		if text == "" {
			continue
		}
		if term, hit := m.matcher.Match(ctx, text); hit {
			return term, true
		}
	}
	return "", false
}

func blockError(scope Scope, term string) error {
	side := "input"
	if scope == ScopeOutput {
		side = "output"
	}
	if term == "" {
		return fmt.Errorf("%w (%s)", ErrUnsafeContent, side)
	}
	return fmt.Errorf("%w (%s: %q)", ErrUnsafeContent, side, term)
}
