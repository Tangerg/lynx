package protocol

import "time"

// ProtocolVersion is the wire version this build implements (doc/API.md §12).
// Current and minimum supported are deliberately equal: this build serves one
// exact version and does not advertise negotiation it cannot perform.
const (
	ProtocolVersion    = "2026-08-17"
	MinProtocolVersion = "2026-08-17"
)

// ProtocolRange is the negotiated wire-version window advertised by discovery
// and transport sidecars. The Protocol qualifier is part of the generated
// contract vocabulary; a generic Range would lose meaning outside this Go package.
//
//nolint:revive // The precise generated wire-shape name intentionally retains its qualifier.
type ProtocolRange struct {
	Current      string `json:"current"`
	MinSupported string `json:"minSupported"`
}

// SupportedProtocolRange returns the exact wire-version window served by this build.
func SupportedProtocolRange() ProtocolRange {
	return ProtocolRange{Current: ProtocolVersion, MinSupported: MinProtocolVersion}
}

// SupportsProtocolVersion reports whether version is a valid date-version
// inside this build's advertised window.
func SupportsProtocolVersion(version string) bool {
	if _, err := time.Parse(time.DateOnly, version); err != nil {
		return false
	}
	return version >= MinProtocolVersion && version <= ProtocolVersion
}
