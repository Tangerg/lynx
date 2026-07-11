package runtime

import (
	"io"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// Dependencies is the fully-assembled collaborator set a [Runtime] facade holds.
// The composition root (bootstrap) builds each collaborator and calls [New]. The
// single *kernel.Engine satisfies the facade's closer, so it is supplied once.
type Dependencies struct {
	Engine       *kernel.Engine
	Turns        turn.Dispatcher
	Conversation *conversation.Messages

	Sessions   sessionsvc.Store
	Interrupts interrupts.Store
	Transcript transcript.Store

	Titles titleGenerator

	Resources []io.Closer
}

// New builds a Runtime facade from already-assembled dependencies. It only
// wires; all construction/validation lives in the bootstrap ring's Assemble,
// which calls New.
func New(d Dependencies) *Runtime {
	return &Runtime{
		turns:      d.Turns,
		closer:     d.Engine,
		resources:  append([]io.Closer(nil), d.Resources...),
		history:    d.Conversation,
		sessions:   d.Sessions,
		interrupts: d.Interrupts,
		transcript: d.Transcript,
		titles:     d.Titles,
	}
}
