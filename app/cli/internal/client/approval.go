package client

import "time"

// ApprovalRule is a remembered decision projected for inspection and removal.
type ApprovalRule struct {
	ID        string
	Rule      string
	Decision  ApprovalDecision
	Scope     RememberScope
	SessionID string
	Workspace string
	CreatedAt time.Time
}
