package runtimeembedded

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

func TestProjectInputReadsTypedAttachmentsAtDispatch(t *testing.T) {
	directory := t.TempDir()
	textPath := filepath.Join(directory, "notes.txt")
	imagePath := filepath.Join(directory, "pixel.png")
	if err := os.WriteFile(textPath, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	image := []byte{0x89, 'P', 'N', 'G'}
	if err := os.WriteFile(imagePath, image, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		loadAttachment: loadAttachmentFile,
		profile: runtimeprofile.Profile{Features: map[runtimeprofile.FeatureName]runtimeprofile.Feature{
			runtimeprofile.FeatureMultimodal: {Enabled: true},
		}},
	}
	blocks, err := runtime.projectInput(t.Context(), agent.Message{
		Text: "prompt",
		Attachments: []agent.Attachment{
			{ID: "text", Kind: agent.AttachmentText, Name: "notes.txt", Path: textPath, MimeType: "text/plain", Size: 5},
			{ID: "image", Kind: agent.AttachmentImage, Name: "pixel.png", Path: imagePath, MimeType: "image/png", Size: int64(len(image))},
		},
	})
	if err != nil {
		t.Fatalf("projectInput: %v", err)
	}
	if len(blocks) != 3 || blocks[0].Type != protocol.ContentBlockText || blocks[0].Text != "prompt" ||
		blocks[1].Type != protocol.ContentBlockText || blocks[2].Type != protocol.ContentBlockImage ||
		blocks[2].Data != base64.StdEncoding.EncodeToString(image) {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestProjectInputRejectsImagesBeforeReadingWithoutMultimodalCapability(t *testing.T) {
	t.Parallel()
	reads := 0
	runtime := &Runtime{loadAttachment: func(context.Context, string, int64) ([]byte, error) {
		reads++
		return []byte("image"), nil
	}}
	blocks, err := runtime.projectInput(t.Context(), agent.Message{Attachments: []agent.Attachment{{
		ID: "image", Kind: agent.AttachmentImage, Name: "image.png", Path: "/image.png",
		MimeType: "image/png", Size: 5,
	}}})
	if err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("projectInput error = %v, want ErrIncompatibleRuntime", err)
	}
	if blocks != nil || reads != 0 {
		t.Fatalf("projectInput = (%+v, %v), want no blocks or attachment reads", blocks, reads)
	}
}

func TestLoadAttachmentFileEnforcesTheReadLimit(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
	}{
		{name: "at limit", content: "12345678"},
		{name: "over limit", content: "123456789", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "attachment.txt")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}

			data, err := loadAttachmentFile(t.Context(), path, 8)
			if !test.wantError {
				if err != nil || string(data) != test.content {
					t.Fatalf("loadAttachmentFile = (%q, %v), want (%q, nil)", data, err, test.content)
				}
				return
			}
			var sizeError attachmentTooLargeError
			if !errors.As(err, &sizeError) || sizeError.maximumBytes != 8 {
				t.Fatalf("loadAttachmentFile error = %v, want 8-byte attachmentTooLargeError", err)
			}
			if data != nil {
				t.Fatalf("loadAttachmentFile data = %q, want nil", data)
			}
		})
	}
}

func TestLoadAttachmentFileHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	data, err := loadAttachmentFile(ctx, path, 8)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("loadAttachmentFile error = %v, want context.Canceled", err)
	}
	if data != nil {
		t.Fatalf("loadAttachmentFile data = %q, want nil", data)
	}
}

func TestProjectContentCreatesHonestDurableImageReference(t *testing.T) {
	text, attachments, err := projectContent("item_1", []protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: "hello"},
		{Type: protocol.ContentBlockImage, Mime: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("image"))},
	})
	if err != nil {
		t.Fatalf("projectContent: %v", err)
	}
	if text != "hello" || len(attachments) != 1 || attachments[0].Path != "" || attachments[0].MimeType != "image/png" {
		t.Fatalf("content = (%q, %+v)", text, attachments)
	}
	if err := attachments[0].Validate(); err != nil {
		t.Fatalf("durable image: %v", err)
	}
}
