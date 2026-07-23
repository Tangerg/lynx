package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// utilityRoleLoader is the boot-time load view of the utility-role store
// (persistence save belongs to the capabilities coordinator's SetUtilityRole).
type utilityRoleLoader interface {
	LoadUtilityRole(ctx context.Context) (provider, model string, err error)
}

// loadUtilityRole reads the persisted startup assignment. Runtime mutation and
// client resolution are owned by their respective application and adapter types.
func loadUtilityRole(ctx context.Context, loader utilityRoleLoader) (modelrole.Role, error) {
	var role modelrole.Role
	if loader != nil {
		p, m, err := loader.LoadUtilityRole(ctx)
		if err != nil {
			return modelrole.Role{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
		role, err = modelrole.New(p, m)
		if err != nil {
			return modelrole.Role{}, fmt.Errorf("bootstrap: load utility role: %w", err)
		}
	}
	return role, nil
}
