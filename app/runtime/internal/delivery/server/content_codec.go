package server

import (
	"encoding/base64"
	"fmt"
	"mime"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type encodedContent struct {
	kind protocol.ContentBlockType
	text string
	mime string
	data string
}

type contentDecodeError struct {
	field  string
	detail string
}

func decodeContent(encoded encodedContent) (transcript.ContentBlock, *contentDecodeError) {
	switch encoded.kind {
	case protocol.ContentBlockText:
		if encoded.text == "" {
			return transcript.ContentBlock{}, &contentDecodeError{field: "text", detail: "must not be empty"}
		}
		if encoded.mime != "" || encoded.data != "" {
			return transcript.ContentBlock{}, &contentDecodeError{field: "type", detail: "text content cannot carry mime or data"}
		}
		return transcript.ContentBlock{Kind: transcript.TextContent, Text: encoded.text}, nil
	case protocol.ContentBlockImage:
		if encoded.mime == "" {
			return transcript.ContentBlock{}, &contentDecodeError{field: "mime", detail: "is required for image content"}
		}
		if encoded.data == "" {
			return transcript.ContentBlock{}, &contentDecodeError{field: "data", detail: "is required for image content"}
		}
		if encoded.text != "" {
			return transcript.ContentBlock{}, &contentDecodeError{field: "type", detail: "image content cannot carry text"}
		}
		mediaType, _, err := mime.ParseMediaType(encoded.mime)
		if err != nil || !strings.HasPrefix(mediaType, "image/") {
			return transcript.ContentBlock{}, &contentDecodeError{
				field: "mime", detail: fmt.Sprintf("must be a supported image MIME, got %q", encoded.mime),
			}
		}
		data, err := base64.StdEncoding.DecodeString(encoded.data)
		if err != nil {
			return transcript.ContentBlock{}, &contentDecodeError{field: "data", detail: "must be valid base64"}
		}
		return transcript.ContentBlock{Kind: transcript.ImageContent, MediaType: mediaType, Bytes: data}, nil
	default:
		return transcript.ContentBlock{}, &contentDecodeError{field: "type", detail: "must be text or image"}
	}
}

func encodeContent(block transcript.ContentBlock) (encodedContent, error) {
	switch block.Kind {
	case transcript.TextContent:
		return encodedContent{kind: protocol.ContentBlockText, text: block.Text}, nil
	case transcript.ImageContent:
		return encodedContent{
			kind: protocol.ContentBlockImage,
			mime: block.MediaType,
			data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return encodedContent{}, fmt.Errorf("unknown content kind %d", block.Kind)
	}
}
