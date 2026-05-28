package coreapi

import (
	"context"
	"time"
)

// AttachmentsAPI is the attachments.* method group. The actual
// binary upload is the protocol's ONE carve-out from JSON-RPC — it
// goes through transport-specific binary channels (HTTP PUT, Wails
// native binding, in-process []byte). See API.md §5.4.
type AttachmentsAPI interface {
	// CreateUploadURL hands the client back a transport-specific
	// upload target. For HTTP it's a presigned URL; for InProcess
	// it's a no-op URL (data is passed via a sibling Go binding).
	CreateUploadURL(ctx context.Context, in CreateUploadURLIn) (*CreateUploadURLOut, error)

	// DeleteAttachment removes one by id.
	DeleteAttachment(ctx context.Context, id string) error
}

// CreateUploadURLIn is the attachments.createUploadUrl request.
type CreateUploadURLIn struct {
	Filename string `json:"filename"`
	Mime     string `json:"mime"`
	Size     int64  `json:"size"`
}

// CreateUploadURLOut is the result.
type CreateUploadURLOut struct {
	UploadURL    string    `json:"uploadUrl"`
	AttachmentID string    `json:"attachmentId"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
