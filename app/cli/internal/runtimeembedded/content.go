package runtimeembedded

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const maximumAttachmentBytes = 20 << 20

var readFile = os.ReadFile

func (r *Runtime) projectInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	blocks := make([]protocol.ContentBlock, 0, 1+len(message.Attachments))
	if message.Text != "" {
		blocks = append(blocks, protocol.ContentBlock{Type: protocol.ContentBlockText, Text: message.Text})
	}
	for _, attachment := range message.Attachments {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		data, err := r.readFile(attachment.Path)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", attachment.Name, err)
		}
		if len(data) > maximumAttachmentBytes {
			return nil, fmt.Errorf("read attachment %q: file exceeds %d bytes", attachment.Name, maximumAttachmentBytes)
		}
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		switch attachment.Kind {
		case agent.AttachmentText:
			blocks = append(blocks, protocol.ContentBlock{
				Type: protocol.ContentBlockText,
				Text: fmt.Sprintf("--- attached file: %q ---\n%s\n--- end attached file ---", attachment.Name, data),
			})
		case agent.AttachmentImage:
			blocks = append(blocks, protocol.ContentBlock{
				Type: protocol.ContentBlockImage, Mime: attachment.MimeType,
				Data: base64.StdEncoding.EncodeToString(data),
			})
		default:
			return nil, fmt.Errorf("attachment %q has unsupported kind %q", attachment.Name, attachment.Kind)
		}
	}
	return blocks, nil
}

func projectContent(itemID string, content []protocol.ContentBlock) (string, []agent.Attachment, error) {
	textParts := make([]string, 0, len(content))
	attachments := make([]agent.Attachment, 0, len(content))
	for index, block := range content {
		switch block.Type {
		case protocol.ContentBlockText:
			textParts = append(textParts, block.Text)
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(block.Data)
			if err != nil {
				return "", nil, fmt.Errorf("item %s image %d: decode base64: %w", itemID, index+1, err)
			}
			name := "image"
			if extensions, _ := mime.ExtensionsByType(block.Mime); len(extensions) != 0 {
				name += extensions[0]
			} else if subtype := strings.TrimPrefix(block.Mime, "image/"); subtype != block.Mime && subtype != "" {
				name += "." + filepath.Base(subtype)
			}
			attachments = append(attachments, agent.Attachment{
				ID: fmt.Sprintf("%s:image:%d", itemID, index), Kind: agent.AttachmentImage,
				Name: name, MimeType: block.Mime, Size: int64(len(data)),
			})
		default:
			return "", nil, fmt.Errorf("item %s content %d has unsupported type %q", itemID, index+1, block.Type)
		}
	}
	return strings.Join(textParts, "\n\n"), attachments, nil
}
