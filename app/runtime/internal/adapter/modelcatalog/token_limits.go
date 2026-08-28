package modelcatalog

import (
	"fmt"

	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

// LookupTokenLimits maps the static catalog envelope for one exact model into
// the Runtime's immutable Domain value. A catalog miss is not an error because
// configured compatible endpoints may legitimately expose private model IDs.
func LookupTokenLimits(selection modelref.Selection) (modelref.TokenLimits, bool, error) {
	if err := selection.Validate(); err != nil {
		return modelref.TokenLimits{}, false, fmt.Errorf("modelcatalog: token-limit selection: %w", err)
	}
	if !selection.Configured() {
		return modelref.TokenLimits{}, false, nil
	}
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return modelref.TokenLimits{}, false, nil
	}
	limits, err := modelref.NewTokenLimits(
		entry.Limits.ContextWindow,
		entry.Limits.MaxInputTokens,
		entry.Limits.MaxOutputTokens,
	)
	if err != nil {
		return modelref.TokenLimits{}, false, fmt.Errorf(
			"modelcatalog: token limits for %q/%q: %w",
			selection.Provider(),
			selection.Model(),
			err,
		)
	}
	return limits, true, nil
}
