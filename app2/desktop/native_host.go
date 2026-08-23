package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

type DirectorySelectionType string

const (
	DirectorySelected DirectorySelectionType = "selected"
	DirectoryCanceled DirectorySelectionType = "canceled"
)

type DirectorySelection struct {
	Type DirectorySelectionType `json:"type"`
	Path string                 `json:"path,omitempty"`
}

type ImageSaveType string

const (
	ImageSaved    ImageSaveType = "saved"
	ImageCanceled ImageSaveType = "canceled"
)

type ImageSaveResult struct {
	Type ImageSaveType `json:"type"`
}

type WindowChrome struct {
	ControlsCentreY   float64 `json:"controlsCentreY"`
	ControlsInlineEnd float64 `json:"controlsInlineEnd"`
	Measured          bool    `json:"measured"`
}

type nativeWindow interface {
	NativeWindow() unsafe.Pointer
}

type directoryPicker interface {
	ChooseDirectory() (string, error)
}

type imageSaver interface {
	SaveImage(string, []byte) (bool, error)
}

// NativeHost exposes only capabilities that truly require the desktop shell.
type NativeHost struct {
	window nativeWindow
	picker directoryPicker
	saver  imageSaver
}

func newNativeHost(window nativeWindow, picker directoryPicker, saver imageSaver) (*NativeHost, error) {
	if window == nil || picker == nil || saver == nil {
		return nil, errors.New("native host: window, directory picker, and image saver are required")
	}
	return &NativeHost{window: window, picker: picker, saver: saver}, nil
}

func (host *NativeHost) WindowChrome() WindowChrome {
	centre, inlineEnd, measured := nativeWindowChrome(host.window.NativeWindow())
	return WindowChrome{ControlsCentreY: centre, ControlsInlineEnd: inlineEnd, Measured: measured}
}

func (host *NativeHost) ChooseDirectory() (DirectorySelection, error) {
	selected, err := host.picker.ChooseDirectory()
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("native host: choose directory: %w", err)
	}
	if selected == "" {
		return DirectorySelection{Type: DirectoryCanceled}, nil
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("native host: resolve selected directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return DirectorySelection{}, fmt.Errorf("native host: inspect selected directory: %w", err)
	}
	if !info.IsDir() {
		return DirectorySelection{}, fmt.Errorf("native host: selected path %q is not a directory", absolute)
	}
	return DirectorySelection{Type: DirectorySelected, Path: filepath.Clean(absolute)}, nil
}

func (host *NativeHost) SaveImage(source string) (ImageSaveResult, error) {
	extension, contents, err := decodeInlineImage(source)
	if err != nil {
		return ImageSaveResult{}, fmt.Errorf("native host: validate image: %w", err)
	}
	saved, err := host.saver.SaveImage(suggestedImageName(extension, time.Now()), contents)
	if err != nil {
		return ImageSaveResult{}, fmt.Errorf("native host: save image: %w", err)
	}
	if !saved {
		return ImageSaveResult{Type: ImageCanceled}, nil
	}
	return ImageSaveResult{Type: ImageSaved}, nil
}

const maxInlineImageBytes = 32 << 20

var imageExtensions = map[string]string{
	"image/avif": "avif", "image/gif": "gif", "image/jpeg": "jpg",
	"image/png": "png", "image/svg+xml": "svg", "image/webp": "webp",
}

func decodeInlineImage(source string) (string, []byte, error) {
	if len(source) > maxInlineImageBytes*2 || !strings.HasPrefix(strings.ToLower(source), "data:") {
		return "", nil, errors.New("source must be a bounded inline data URL")
	}
	comma := strings.IndexByte(source, ',')
	if comma < len("data:") {
		return "", nil, errors.New("inline data URL has no payload")
	}
	metadata := strings.Split(source[len("data:"):comma], ";")
	mimeType := strings.ToLower(strings.TrimSpace(metadata[0]))
	extension, allowed := imageExtensions[mimeType]
	if !allowed {
		return "", nil, fmt.Errorf("unsupported image media type %q", mimeType)
	}
	base64Encoded := false
	for _, parameter := range metadata[1:] {
		parameter = strings.TrimSpace(parameter)
		if strings.EqualFold(parameter, "base64") {
			base64Encoded = true
			continue
		}
		if parameter != "" {
			return "", nil, fmt.Errorf("unsupported data URL parameter %q", parameter)
		}
	}
	payload := source[comma+1:]
	var contents []byte
	var err error
	if base64Encoded {
		contents, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		contents = []byte(decoded)
	}
	if err != nil {
		return "", nil, fmt.Errorf("decode inline image: %w", err)
	}
	if len(contents) == 0 || len(contents) > maxInlineImageBytes {
		return "", nil, errors.New("inline image is empty or exceeds the size limit")
	}
	return extension, contents, nil
}

func suggestedImageName(extension string, now time.Time) string {
	return fmt.Sprintf("Lyra Image %s.%s", now.Format("2006-01-02 at 15.04.05"), extension)
}
