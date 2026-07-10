package runtime

import (
	"context"
)

type titleGenerator interface {
	Generate(ctx context.Context, firstMessage string) (string, error)
}

// GenerateTitle derives a short session title from a conversation's opening
// user message — auto-naming an untitled session (the wire Session.title).
// Best-effort: returns "" (no error) when titling isn't possible. Lives here,
// like [Runtime.ProbeProvider], because the runtime owns the maintenance LLM
// client; the delivery layer triggers it off a finished root run.
func (r *Runtime) GenerateTitle(ctx context.Context, firstMessage string) (string, error) {
	if r.titles == nil {
		return "", nil
	}
	return r.titles.Generate(ctx, firstMessage)
}
