package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// EventListener is the [event.Event] subscriber capability — runtime
// counterpart to the marker interfaces in [core]. It lives in runtime
// because [event.Event] is tied to the framework's concrete event types
// and putting it in core would create an import cycle (event → core →
// event). Implementing EventListener registers the value with the
// platform's multicast at boot.
//
// A registered EventListener also satisfies the simpler [event.Listener]
// (which the multicast accepts directly), so the runtime forwards the
// extension straight to [event.Multicast.Add].
type EventListener interface {
	core.Extension

	OnEvent(ctx context.Context, e event.Event)
}
