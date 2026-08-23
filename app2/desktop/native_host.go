package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

type SessionArtifactOpenType string

const (
	SessionArtifactSelected SessionArtifactOpenType = "selected"
	SessionArtifactCanceled SessionArtifactOpenType = "canceled"
)

type SessionArtifactOpenResult struct {
	Type     SessionArtifactOpenType `json:"type"`
	Contents string                  `json:"contents,omitempty"`
}

type SessionExportSaveType string

const (
	SessionExportSaved    SessionExportSaveType = "saved"
	SessionExportCanceled SessionExportSaveType = "canceled"
)

type SessionExportSaveResult struct {
	Type SessionExportSaveType `json:"type"`
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

type sessionDocumentTransfer interface {
	OpenArtifact(int64) ([]byte, bool, error)
	SaveExport(string, []byte) (bool, error)
}

// NativeHost exposes only capabilities that truly require the desktop shell.
type NativeHost struct {
	window    nativeWindow
	picker    directoryPicker
	saver     imageSaver
	documents sessionDocumentTransfer
}

func newNativeHost(
	window nativeWindow,
	picker directoryPicker,
	saver imageSaver,
	documents sessionDocumentTransfer,
) (*NativeHost, error) {
	if window == nil || picker == nil || saver == nil || documents == nil {
		return nil, errors.New("native host: window, pickers, and savers are required")
	}
	return &NativeHost{
		window: window, picker: picker, saver: saver, documents: documents,
	}, nil
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

const maxSessionDocumentBytes = 256 << 20

var sessionIdentityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func (host *NativeHost) OpenSessionArtifact() (SessionArtifactOpenResult, error) {
	contents, selected, err := host.documents.OpenArtifact(maxSessionDocumentBytes)
	if err != nil {
		return SessionArtifactOpenResult{}, fmt.Errorf("native host: open session artifact: %w", err)
	}
	if !selected {
		return SessionArtifactOpenResult{Type: SessionArtifactCanceled}, nil
	}
	if !json.Valid(contents) {
		return SessionArtifactOpenResult{}, errors.New("native host: selected session artifact is not JSON")
	}
	return SessionArtifactOpenResult{
		Type: SessionArtifactSelected, Contents: string(contents),
	}, nil
}

func (host *NativeHost) SaveSessionExport(
	sessionID string,
	format string,
	contents string,
) (SessionExportSaveResult, error) {
	if !sessionIdentityPattern.MatchString(sessionID) {
		return SessionExportSaveResult{}, errors.New("native host: invalid session identity")
	}
	extension := ""
	switch format {
	case "json":
		extension = ".json"
		if !json.Valid([]byte(contents)) {
			return SessionExportSaveResult{}, errors.New("native host: JSON export is invalid")
		}
	case "md":
		extension = ".md"
	default:
		return SessionExportSaveResult{}, errors.New("native host: invalid session export format")
	}
	if len(contents) == 0 || len(contents) > maxSessionDocumentBytes {
		return SessionExportSaveResult{}, errors.New("native host: session export is empty or too large")
	}
	saved, err := host.documents.SaveExport("lyra-"+sessionID+extension, []byte(contents))
	if err != nil {
		return SessionExportSaveResult{}, fmt.Errorf("native host: save session export: %w", err)
	}
	if !saved {
		return SessionExportSaveResult{Type: SessionExportCanceled}, nil
	}
	return SessionExportSaveResult{Type: SessionExportSaved}, nil
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
