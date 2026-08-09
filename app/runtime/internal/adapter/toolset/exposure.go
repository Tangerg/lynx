package toolset

import (
	"slices"

	toolcontract "github.com/Tangerg/lynx/tool"
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
func (manifest Manifest) Clone() Manifest {
	return Manifest{
		Visible:  slices.Clone(manifest.Visible),
		Deferred: slices.Clone(manifest.Deferred),
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

func (builder *manifestBuilder) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			builder.visible = append(builder.visible, candidate)
		}
	}
}

func (builder *manifestBuilder) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			builder.deferred = append(builder.deferred, candidate)
		}
	}
}

func (builder manifestBuilder) manifest() Manifest {
	return Manifest{
		Visible:  slices.Clone(builder.visible),
		Deferred: slices.Clone(builder.deferred),
	}
}
