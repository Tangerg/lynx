// Package sessiontransfer defines the consumer-owned port for portable runtime
// session artifacts. The artifact body stays opaque to the CLI domain.
package sessiontransfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type Format string

const (
	Markdown Format = "md"
	JSON     Format = "json"
)

func ParseFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "md", "markdown":
		return Markdown, nil
	case "json":
		return JSON, nil
	default:
		return "", fmt.Errorf("export format %q is unsupported; use markdown or json", strings.TrimSpace(value))
	}
}

func (f Format) Extension() string {
	switch f {
	case Markdown:
		return ".md"
	case JSON:
		return ".json"
	default:
		return ""
	}
}

func (f Format) Validate() error {
	if f != Markdown && f != JSON {
		return fmt.Errorf("session document format %q is invalid", f)
	}
	return nil
}

// Document is an immutable runtime-authored export. JSON documents are
// round-trippable; Markdown documents are human-readable projections only.
type Document struct {
	format Format
	body   []byte
}

func NewDocument(format Format, body []byte) (Document, error) {
	if err := format.Validate(); err != nil {
		return Document{}, err
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return Document{}, errors.New("session document is empty")
	}
	if !utf8.Valid(body) {
		return Document{}, errors.New("session document is not valid UTF-8")
	}
	if format == JSON && !json.Valid(body) {
		return Document{}, errors.New("session artifact is not valid JSON")
	}
	return Document{format: format, body: slices.Clone(body)}, nil
}

func (d Document) Format() Format { return d.format }
func (d Document) Bytes() []byte  { return slices.Clone(d.body) }

func (d Document) Validate() error {
	_, err := NewDocument(d.format, d.body)
	return err
}

func (d Document) Importable() bool {
	return d.format == JSON && d.Validate() == nil
}

type ExportRequest struct {
	SessionID string
	Format    Format
}

func (e ExportRequest) Validate() error {
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("export session: session id is empty")
	}
	if err := e.Format.Validate(); err != nil {
		return fmt.Errorf("export session: %w", err)
	}
	return nil
}

type ImportRequest struct{ Artifact Document }

func (i ImportRequest) Validate() error {
	if err := i.Artifact.Validate(); err != nil {
		return fmt.Errorf("import session: %w", err)
	}
	if !i.Artifact.Importable() {
		return errors.New("import session: only JSON session artifacts are importable")
	}
	return nil
}

type Service interface {
	ExportSession(context.Context, ExportRequest) (Document, error)
	ImportSession(context.Context, ImportRequest) (agent.Session, error)
}
