package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// utilityRoleLoader is the boot-time load view of the utility-role store
// (persistence save belongs to the capabilities coordinator's SetUtilityRole).
type utilityRoleLoader interface {
	LoadUtilityRole(ctx context.Context) (modelref.Selection, error)
}

// loadUtilityRole reads the persisted startup assignment. Runtime mutation and
// client resolution are owned by their respective application and adapter types.
func loadUtilityRole(ctx context.Context, loader utilityRoleLoader) (modelref.Selection, error) {
	var role modelref.Selection
	if loader != nil {
		loaded, err := loader.LoadUtilityRole(ctx)
		if err != nil {
			return modelref.Selection{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
		role = loaded
	}
	return role, nil
}
