package protocol

import (
	"context"
	"time"
)

// Attachments is the attachments.* method group (API.md §7.7). Binary
// upload is the protocol's one carve-out from JSON-RPC — it goes
// through transport-specific binary channels. Gated on features.attachments.
type Attachments interface {
	CreateUploadURL(ctx context.Context, in CreateUploadURLRequest) (*CreateUploadURLResponse, error)
	GetAttachment(ctx context.Context, attachmentID string) (*Attachment, error)
	DeleteAttachment(ctx context.Context, attachmentID string) error
}

// Attachment is one stored attachment (API.md §4.10).
type Attachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateUploadURLRequest — attachments.createUploadUrl body.
type CreateUploadURLRequest struct {
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	SizeBytes int64  `json:"sizeBytes"`
}

// CreateUploadURLResponse — attachments.createUploadUrl result.
type CreateUploadURLResponse struct {
	AttachmentID string    `json:"attachmentId"`
	UploadURL    string    `json:"uploadUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
