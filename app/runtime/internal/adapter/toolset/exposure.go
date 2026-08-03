package toolset

import (
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// resolvedToolset is the one real visibility decision made while assembling a
// Run: direct tools enter the initial model manifest, while deferred tools stay
// executable but are loaded through search_tools. Unavailable tools are simply
// never added; there is no synthetic visibility state for them.
type resolvedToolset struct {
	all      []toolcontract.Tool
	deferred []toolcontract.Tool
}

func (s *resolvedToolset) direct(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			s.all = append(s.all, candidate)
		}
	}
}

func (s *resolvedToolset) deferTools(tools ...toolcontract.Tool) {
	for _, candidate := range tools {
		if candidate != nil {
			s.all = append(s.all, candidate)
			s.deferred = append(s.deferred, candidate)
		}
	}
}

// useApplyPatch selects one mutation vocabulary for the complete Run. The
// mapping follows the native tool dialects of the five compared agents:
// modern GPT/Codex and Grok use apply_patch regardless of provider route;
// Claude, Kimi, legacy GPT-4, OSS models, and unknown models use edit + write.
func useApplyPatch(selection modelref.Selection) bool {
	model := strings.ToLower(selection.Model())
	if strings.Contains(model, "grok") {
		return true
	}
	return strings.Contains(model, "gpt-") &&
		!strings.Contains(model, "gpt-4") &&
		!strings.Contains(model, "oss")
}
