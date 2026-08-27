package toolset

import (
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Manifest is one Run's frozen, framework-neutral model Tool surface. Visible
// Tools enter the initial model manifest. Deferred Tools are already executable
// authority but remain hidden until the discovery Tool advertises their exact
// names. The slices never overlap.
type Manifest struct {
	Visible  []toolcontract.Tool
	Deferred []toolcontract.Tool
}

// Clone returns an ownership-isolated manifest. Tool implementations are
// immutable capabilities; only the containing slices require isolation.
func (m Manifest) Clone() Manifest {
	return Manifest{
		Visible:  slices.Clone(m.Visible),
		Deferred: slices.Clone(m.Deferred),
	}
}

// manifestBuilder owns the one real visibility decision made while assembling a
// Run: direct tools enter the initial model manifest, while deferred tools stay
// executable but are loaded through search_tools. Unavailable tools are simply
// never added; there is no synthetic visibility state for them.
type manifestBuilder struct {
	visible  []toolcontract.Tool
	deferred []toolcontract.Tool
}

func (m *manifestBuilder) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.visible = append(m.visible, candidate)
		}
	}
}

func (m *manifestBuilder) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			m.deferred = append(m.deferred, candidate)
		}
	}
}

func (m manifestBuilder) manifest() Manifest {
	return Manifest{
		Visible:  slices.Clone(m.visible),
		Deferred: slices.Clone(m.deferred),
	}
}
