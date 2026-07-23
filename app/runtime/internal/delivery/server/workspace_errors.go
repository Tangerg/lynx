package server

import (
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// wireWorkspaceError is the sole translation from workspace use-case failures
// to the JSON-RPC error vocabulary. The application never imports protocol.
func wireWorkspaceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspaceapp.ErrCwdUnavailable):
		return fmt.Errorf("%w: %w", protocol.ErrCwdUnavailable, err)
	case errors.Is(err, workspaceapp.ErrPathOutsideRoot):
		return protocol.ErrPathOutsideRoot
	case errors.Is(err, workspaceapp.ErrPathRequired),
		errors.Is(err, workspaceapp.ErrInvalidFileRange),
		errors.Is(err, workspaceapp.ErrGrepQueryMissing),
		errors.Is(err, workspaceapp.ErrFileListTooLarge),
		errors.Is(err, workspaceapp.ErrPageLimit),
		errors.Is(err, workspaceapp.ErrPageCursor),
		errors.Is(err, workspaceapp.ErrVCSBaseUnknown):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	case errors.Is(err, workspaceapp.ErrVCSUnavailable):
		return protocol.ErrVcsUnavailable
	default:
		return err
	}
}
