package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

const defaultDownloadMaxBytes = 50 << 20

type downloadRequest struct {
	URL       string `json:"url" jsonschema:"required" jsonschema_description:"HTTP or HTTPS URL to download."`
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
	client  *http.Client
}

func newDownloadTool(workdir string) chat.Tool {
	return &downloadTool{workdir: workdir, client: http.DefaultClient}
}

func (t *downloadTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "download",
		Description: "Download an HTTP(S) URL to a local file. Parent directories are created. Existing files are not overwritten unless overwrite=true.",
		InputSchema: downloadSchema,
	}
}

func (t *downloadTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req downloadRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.FilePath, true
}

func (t *downloadTool) MutatedPaths(arguments string) ([]string, error) {
	var req downloadRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	if req.FilePath == "" {
		return nil, nil
	}
	return []string{req.FilePath}, nil
}

func (t *downloadTool) Call(ctx context.Context, arguments string) (string, error) {
	var req downloadRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("download: parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return "", errors.New("download: file_path must not be empty")
	}
	u, err := parseDownloadURL(req.URL)
	if err != nil {
		return "", err
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultDownloadMaxBytes
	}
	path := resolveAbs(t.workdir, req.FilePath)
	if !req.Overwrite {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("download: %s already exists; pass overwrite=true to replace it", req.FilePath)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("download: build request: %w", err)
	}
	res, err := t.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("download: GET %s returned %s", u.Redacted(), res.Status)
	}
	if res.ContentLength > maxBytes {
		return "", fmt.Errorf("download: response is %d bytes, over max_bytes=%d", res.ContentLength, maxBytes)
	}
	n, err := writeDownloadedFile(path, res.Body, maxBytes, req.Overwrite)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(downloadResponse{
		FilePath:    req.FilePath,
		Bytes:       n,
		ContentType: res.Header.Get("Content-Type"),
	})
	if err != nil {
		return "", fmt.Errorf("download: marshal: %w", err)
	}
	return string(body), nil
}

func parseDownloadURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("download: url must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("download: invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("download: unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("download: url host must not be empty")
	}
	return u, nil
}

func writeDownloadedFile(path string, body io.Reader, maxBytes int64, overwrite bool) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("download: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return 0, fmt.Errorf("download: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	var written int64
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	written, err = io.Copy(tmp, io.LimitReader(body, maxBytes+1))
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
			return 0, fmt.Errorf("download: %s already exists; pass overwrite=true to replace it", filepath.Base(path))
		}
		return 0, fmt.Errorf("download: link: %w", err)
	}
	return written, nil
}
