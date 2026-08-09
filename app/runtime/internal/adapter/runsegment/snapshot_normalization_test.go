package runsegment

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

func normalizeCapabilities(capabilities run.Capabilities) run.Capabilities {
	capabilities = capabilities.Normalized()
	if len(capabilities.InterruptKinds) == 0 {
		capabilities.InterruptKinds = nil
	}
	return capabilities
}

func timeFromUnixNano(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Unix(0, value.UnixNano()).UTC()
}
