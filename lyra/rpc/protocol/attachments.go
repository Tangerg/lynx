package protocol

import (
	"context"
	"time"
)

// Attachments is the attachments.* method group. The actual
// binary upload is the protocol's ONE carve-out from JSON-RPC — it
// goes through transport-specific binary channels (HTTP PUT, Wails
// native binding, in-process []byte). See API.md §5.4.
type Attachments interface {
	// CreateUploadURL hands the client back a transport-specific
	// upload target. For HTTP it's a presigned URL; for InProcess
	// it's a no-op URL (data is passed via a sibling Go binding).
	CreateUploadURL(ctx context.Context, in CreateUploadURLRequest) (*CreateUploadURLResponse, error)

	// DeleteAttachment removes one by id.
	DeleteAttachment(ctx context.Context, id string) error
}

// CreateUploadURLRequest is the attachments.createUploadUrl request.
type CreateUploadURLRequest struct {
	Filename string `json:"filename"`
	Mime     string `json:"mime"`
	Size     int64  `json:"size"`
}

// CreateUploadURLResponse is the result.
type CreateUploadURLResponse struct {
	UploadURL    string    `json:"uploadUrl"`
	AttachmentID string    `json:"attachmentId"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
