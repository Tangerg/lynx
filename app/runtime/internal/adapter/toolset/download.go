package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	"github.com/Tangerg/lynx/tools"
	"github.com/Tangerg/lynx/tools/httpreq"
)

const (
	defaultDownloadMaxBytes = 50 << 20
	// downloadTimeout bounds the whole GET incl. body read — a hard cap on a
	// slow or endless transfer (resty sets it on the underlying http.Client).
	downloadTimeout = 10 * time.Minute
)

type downloadRequest struct {
	URL       string `json:"url" jsonschema:"required" jsonschema_description:"HTTP or HTTPS URL to download. The host must match the configured download allowlist."`
	FilePath  string `json:"file_path" jsonschema:"required" jsonschema_description:"Destination path — absolute, or relative to the workspace root. Parent directories are created automatically."`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema_description:"Overwrite an existing file. Default false."`
	MaxBytes  int64  `json:"max_bytes,omitempty" jsonschema_description:"Maximum bytes to download. Default 52428800."`
}

type downloadResponse struct {
	FilePath    string `json:"file_path"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
}

var downloadSchema, _ = pkgjson.StringDefSchemaOf(downloadRequest{})

type downloadTool struct {
	workdir string
	allow   httpreq.Allowlist
	client  *resty.Client
}

// newDownloadTool builds the download tool. allow is the SAME host allowlist
// that gates httpreq (a download is an arbitrary-URL GET that also writes to
// disk, so it carries the identical SSRF surface); the caller only registers
// the tool when the allowlist is non-empty.
func newDownloadTool(workdir string, allow httpreq.Allowlist) tools.Tool {
	client := resty.New().SetTimeout(downloadTimeout)
	return &downloadTool{workdir: workdir, allow: allow, client: client}
}

func (t *downloadTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "download",
		Description: "Download an HTTP(S) URL to a local file. The URL host must be in the configured allowlist. Parent directories are created. Existing files are not overwritten unless overwrite=true.",
		InputSchema: json.RawMessage(downloadSchema),
	}
}

func (t *downloadTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var request downloadRequest
	_ = json.Unmarshal([]byte(arguments), &request)
	return request.FilePath, true
}

func (t *downloadTool) MutatedPaths(arguments string) ([]string, error) {
	var request downloadRequest
	if err := json.Unmarshal([]byte(arguments), &request); err != nil {
		return nil, err
	}
	if request.FilePath == "" {
		return nil, nil
	}
	return []string{request.FilePath}, nil
}

func (t *downloadTool) Call(ctx context.Context, arguments string) (string, error) {
	var request downloadRequest
	if err := json.Unmarshal([]byte(arguments), &request); err != nil {
		return "", fmt.Errorf("download: parse arguments: %w", err)
	}
	if request.FilePath == "" {
		return "", errors.New("download: file_path must not be empty")
	}
	parsedURL, err := parseDownloadURL(request.URL)
	if err != nil {
		return "", err
	}
	host := parsedURL.Hostname()
	if !t.allow.Allows(host) {
		return "", fmt.Errorf("download: host %q is not in the allowlist", host)
	}
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultDownloadMaxBytes
	}
	path := resolveAbs(t.workdir, request.FilePath)
	if !request.Overwrite {
		if err := checkDownloadTarget(path, request.FilePath); err != nil {
			return "", err
		}
	}

	response, err := t.client.R().SetContext(ctx).SetDoNotParseResponse(true).Get(parsedURL.String())
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	body := response.RawBody()
	defer body.Close()
	if response.StatusCode() < 200 || response.StatusCode() >= 300 {
		return "", fmt.Errorf("download: GET %s returned %s", parsedURL.Redacted(), response.Status())
	}
	if contentLength := response.RawResponse.ContentLength; contentLength > maxBytes {
		return "", fmt.Errorf("download: response is %d bytes, over max_bytes=%d", contentLength, maxBytes)
	}
	bytesWritten, err := writeDownloadedFile(path, request.FilePath, body, maxBytes, request.Overwrite)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(downloadResponse{
		FilePath:    request.FilePath,
		Bytes:       bytesWritten,
		ContentType: response.Header().Get("Content-Type"),
	})
	if err != nil {
		return "", fmt.Errorf("download: marshal: %w", err)
	}
	return string(out), nil
}

func parseDownloadURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("download: url must not be empty")
	}
	parsedURL, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("download: invalid url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("download: unsupported scheme %q", parsedURL.Scheme)
	}
	if parsedURL.Host == "" {
		return nil, errors.New("download: url host must not be empty")
	}
	return parsedURL, nil
}

func checkDownloadTarget(path, displayPath string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("download: %s already exists; pass overwrite=true to replace it", displayPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("download: stat %s: %w", displayPath, err)
	}
	return nil
}

func writeDownloadedFile(path, displayPath string, body io.Reader, maxBytes int64, overwrite bool) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("download: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return 0, fmt.Errorf("download: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	written, err := io.Copy(tmp, io.LimitReader(body, maxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("download: write: %w", err)
	}
	if written > maxBytes {
		return 0, fmt.Errorf("download: response exceeded max_bytes=%d", maxBytes)
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("download: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("download: close: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return 0, fmt.Errorf("download: chmod: %w", err)
	}
	if overwrite {
		if err := os.Rename(tmpPath, path); err != nil {
			return 0, fmt.Errorf("download: rename: %w", err)
		}
		return written, nil
	}
	if err := os.Link(tmpPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return 0, fmt.Errorf("download: %s already exists; pass overwrite=true to replace it", displayPath)
		}
		return 0, fmt.Errorf("download: link: %w", err)
	}
	return written, nil
}
