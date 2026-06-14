package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
	"github.com/Tangerg/lynx/lyra/internal/domain/approval"
)

// WorkspaceGetApprovalMode returns the current runtime tool-permission stance
// (workspace.getApprovalMode).
func (s *Server) WorkspaceGetApprovalMode(ctx context.Context) (*protocol.ApprovalModeResult, error) {
	m, err := s.rt.Approval().GetMode(ctx)
	if err != nil {
		return nil, err
	}
	return &protocol.ApprovalModeResult{Mode: approvalModeToWire(m)}, nil
}

// WorkspaceSetApprovalMode sets the runtime tool-permission stance
// (workspace.setApprovalMode). plan is the read-only planning stance.
func (s *Server) WorkspaceSetApprovalMode(ctx context.Context, in protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
	mode, ok := approvalModeFromWire(in.Mode)
	if !ok {
		return nil, fmt.Errorf("%w: unknown approval mode %q", protocol.ErrInvalidParams, in.Mode)
	}
	if err := s.rt.Approval().SetMode(ctx, mode); err != nil {
		return nil, err
	}
	return &protocol.ApprovalModeResult{Mode: in.Mode}, nil
}

// approvalModeToWire maps the engine stance to its wire name. An unknown value
// maps to balanced (the documented default execute stance).
func approvalModeToWire(m approval.Mode) protocol.ApprovalMode {
	switch m {
	case approval.ModeSafe:
		return protocol.ApprovalModeSafe
	case approval.ModeYolo:
		return protocol.ApprovalModeYolo
	case approval.ModePlan:
		return protocol.ApprovalModePlan
	default:
		return protocol.ApprovalModeBalanced
	}
}

// approvalModeFromWire maps a wire stance to the engine stance; ok=false for an
// unknown value (the caller raises invalid_params).
func approvalModeFromWire(m protocol.ApprovalMode) (approval.Mode, bool) {
	switch m {
	case protocol.ApprovalModeSafe:
		return approval.ModeSafe, true
	case protocol.ApprovalModeBalanced:
		return approval.ModeBalanced, true
	case protocol.ApprovalModeYolo:
		return approval.ModeYolo, true
	case protocol.ApprovalModePlan:
		return approval.ModePlan, true
	}
	return 0, false
}
