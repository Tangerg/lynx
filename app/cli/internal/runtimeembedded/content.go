package runtimeembedded

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

const maximumAttachmentBytes = 20 << 20

type attachmentTooLargeError struct {
	maximumBytes int64
}

func (e attachmentTooLargeError) Error() string {
	return fmt.Sprintf("file exceeds %d bytes", e.maximumBytes)
}

type attachmentLoader func(context.Context, string, int64) ([]byte, error)

func loadAttachmentFile(ctx context.Context, path string, maximumBytes int64) (_ []byte, err error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	data, err := io.ReadAll(io.LimitReader(contextReader{Context: ctx, Reader: file}, maximumBytes+1))
	if err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if int64(len(data)) > maximumBytes {
		return nil, attachmentTooLargeError{maximumBytes: maximumBytes}
	}
	return data, nil
}

type contextReader struct {
	context.Context
	io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := context.Cause(r.Context); err != nil {
		return 0, err
	}
	return r.Reader.Read(buffer)
}

func (r *Runtime) projectInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if err := message.Validate(); err != nil {
		return nil, err
	}
	if err := r.requireInputCapabilities(message); err != nil {
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
		data, err := r.loadAttachment(ctx, attachment.Path, maximumAttachmentBytes)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", attachment.Name, err)
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

func (r *Runtime) requireInputCapabilities(message agent.Message) error {
	for _, attachment := range message.Attachments {
		if attachment.Kind == agent.AttachmentImage {
			return r.requireFeature(runtimeprofile.FeatureMultimodal)
		}
	}
	return nil
}

func projectContent(itemID string, content []protocol.ContentBlock) (string, []agent.Attachment, error) {
	projected, err := projectContentValue(itemID, content)
	return projected.text, projected.attachments, err
}

func projectAssistantContent(itemID string, content []protocol.ContentBlock) (string, []agent.InlineImage, error) {
	projected, err := projectContentValue(itemID, content)
	return projected.text, projected.images, err
}

type contentProjection struct {
	text        string
	attachments []agent.Attachment
	images      []agent.InlineImage
}

func projectContentValue(itemID string, content []protocol.ContentBlock) (contentProjection, error) {
	textParts := make([]string, 0, len(content))
	attachments := make([]agent.Attachment, 0, len(content))
	images := make([]agent.InlineImage, 0, len(content))
	for index, block := range content {
		switch block.Type {
		case protocol.ContentBlockText:
			textParts = append(textParts, block.Text)
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(block.Data)
			if err != nil {
				return contentProjection{}, fmt.Errorf("item %s image %d: decode base64: %w", itemID, index+1, err)
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
			images = append(images, agent.InlineImage{
				ID: fmt.Sprintf("%s:image:%d", itemID, index), Name: name, MIMEType: block.Mime, Data: data,
			})
		default:
			return contentProjection{}, fmt.Errorf("item %s content %d has unsupported type %q", itemID, index+1, block.Type)
		}
	}
	return contentProjection{
		text: strings.Join(textParts, "\n\n"), attachments: attachments, images: images,
	}, nil
}
