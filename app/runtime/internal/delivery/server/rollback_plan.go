package server

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

type rollbackIntent struct {
	restoreFiles   bool
	restoreHistory bool
}

func rollbackIntentFromWire(in protocol.RollbackSessionRequest) (rollbackIntent, error) {
	restoreType := in.RestoreType
	if restoreType == "" {
		restoreType = protocol.RestoreHistory
	}

	switch restoreType {
	case protocol.RestoreFiles:
		if in.ToRunID == "" {
			return rollbackIntent{}, fmt.Errorf("%w: restoreType %q requires toRunId", protocol.ErrInvalidParams, restoreType)
		}
		return rollbackIntent{restoreFiles: true}, nil
	case protocol.RestoreHistory:
		return rollbackIntent{restoreHistory: true}, nil
	case protocol.RestoreBoth:
		if in.ToRunID == "" {
			return rollbackIntent{}, fmt.Errorf("%w: restoreType %q requires toRunId", protocol.ErrInvalidParams, restoreType)
		}
		return rollbackIntent{restoreFiles: true, restoreHistory: true}, nil
	default:
		return rollbackIntent{}, fmt.Errorf("%w: unknown restoreType %q", protocol.ErrInvalidParams, restoreType)
	}
}
