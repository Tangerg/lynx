package toolset

import toolcontract "github.com/Tangerg/lynx/tool"

// resolution is the one real visibility decision made while assembling a
// Run: direct tools enter the initial model manifest, while deferred tools stay
// executable but are loaded through search_tools. Unavailable tools are simply
// never added; there is no synthetic visibility state for them.
type resolution struct {
	all      []toolcontract.Tool
	deferred []toolcontract.Tool
}

func (s *resolution) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			s.all = append(s.all, candidate)
		}
	}
}

func (s *resolution) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			s.all = append(s.all, candidate)
			s.deferred = append(s.deferred, candidate)
		}
	}
}
