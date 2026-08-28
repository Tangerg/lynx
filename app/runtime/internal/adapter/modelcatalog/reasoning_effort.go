package modelcatalog

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
)

// ErrUnsupportedReasoningEffort reports an explicit intensity absent from the
// exact known model's advertised vocabulary.
var ErrUnsupportedReasoningEffort = errors.New("modelcatalog: unsupported reasoning effort")

// AdmitSelection validates the exact model-owned reasoning vocabulary for
// catalog models. A catalog miss remains admissible because compatible
// endpoints may expose private models whose capabilities are unavailable
// locally; their provider remains the execution authority.
func (Capabilities) AdmitSelection(selection modelref.Selection) error {
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("modelcatalog: selection: %w", err)
	}
	effort := selection.ReasoningEffort()
	if effort == "" {
		return nil
	}
	entry, found := catalog.Default.Lookup(selection.Provider(), selection.Model())
	if !found {
		return nil
	}
	if entry.Reasoning.Supported && slices.Contains(entry.Reasoning.Levels, effort) {
		return nil
	}
	return fmt.Errorf(
		"%w: %w: model %q/%q does not advertise %q (available: %v)",
		modelref.ErrUnsupported,
		ErrUnsupportedReasoningEffort,
		selection.Provider(),
		selection.Model(),
		effort,
		entry.Reasoning.Levels,
	)
}
