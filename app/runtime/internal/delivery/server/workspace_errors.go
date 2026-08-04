package server

import (
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func workspaceRefPath(ref *protocol.WorkspaceRef) string {
	if ref == nil {
		return ""
	}
	return ref.Path
}

func workspaceRefFromPath(path string) *protocol.WorkspaceRef {
	if path == "" {
		return nil
	}
	return &protocol.WorkspaceRef{Path: path}
}

func workspacePathPatch(ref *protocol.WorkspaceRef) *string {
	if ref == nil {
		return nil
	}
	path := ref.Path
	return &path
}

// wireWorkspaceError is the sole translation from workspace use-case failures
// to the JSON-RPC error vocabulary. The application never imports protocol.
func wireWorkspaceError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspaceapp.ErrCwdUnavailable):
		return fmt.Errorf("%w: %w", protocol.ErrWorkspaceUnavailable, err)
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
	// A capability this build never assembled is capability_not_negotiated, the
	// same answer discovery's feature map implies (API.md §9). The dispatcher's
	// rule refuses these before they get here; this mapping keeps the sentinel
	// from surfacing raw on another path into the same workspace use case.
	case errors.Is(err, workspaceapp.ErrMemoryUnavailable):
		return fmt.Errorf("%w: %w", protocol.ErrCapabilityNotNeg, err)
	default:
		return err
	}
}
