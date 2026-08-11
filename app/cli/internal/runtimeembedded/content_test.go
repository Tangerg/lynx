package runtimeembedded

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	runtime := &Runtime{readFile: readFile}
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
