package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var imageExtensionByMIME = map[string]string{
	"image/avif":    "avif",
	"image/gif":     "gif",
	"image/jpeg":    "jpg",
	"image/jpg":     "jpg",
	"image/png":     "png",
	"image/svg+xml": "svg",
	"image/webp":    "webp",
}

// decodeInlineImage accepts the same restricted inline-image family rendered by the
// frontend. Remote URLs never cross this boundary, and malformed or non-image data is
// rejected before a native dialog is opened.
func decodeInlineImage(source string) (mimeType, extension string, contents []byte, err error) {
	if !strings.HasPrefix(strings.ToLower(source), "data:") {
		return "", "", nil, errors.New("source is not an inline data URL")
	}
	comma := strings.IndexByte(source, ',')
	if comma < len("data:") {
		return "", "", nil, errors.New("inline data URL has no payload")
	}
	metadata := strings.Split(source[len("data:"):comma], ";")
	mimeType = strings.ToLower(strings.TrimSpace(metadata[0]))
	extension, ok := imageExtensionByMIME[mimeType]
	if !ok {
		return "", "", nil, fmt.Errorf("unsupported image media type %q", mimeType)
	}

	base64Encoded := false
	for _, parameter := range metadata[1:] {
		if strings.EqualFold(strings.TrimSpace(parameter), "base64") {
			base64Encoded = true
		}
	}
	payload := source[comma+1:]
	if base64Encoded {
		contents, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		contents = []byte(decoded)
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("decode inline image: %w", err)
	}
	if len(contents) == 0 {
		return "", "", nil, errors.New("inline image is empty")
	}
	return mimeType, extension, contents, nil
}

func suggestedImageFilename(extension string) string {
	stamp := time.Now().Format("2006-01-02 at 15.04.05")
	return fmt.Sprintf("Lyra Image %s.%s", stamp, extension)
}
