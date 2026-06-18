package media

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/pkg/mime"
)

// Media wraps an opaque payload (Data) with its MIME type and metadata.
// Data is intentionally typed as any so a single struct can hold raw
// bytes, base64 strings, URLs, or [io.Reader]s — callers extract the
// concrete shape with [Media.DataAsBytes] / [Media.DataAsString].
type Media struct {
	// ID is an optional identifier — set when the caller wants to
	// correlate this payload across logs / audit trails.
	ID string `json:"id,omitempty"`

	// Name is an optional display name (filename, label, ...).
	Name string `json:"name,omitempty"`

	// MimeType identifies the content type. Required.
	MimeType *mime.MIME `json:"mime_type,omitempty"`

	// Data is the actual payload — []byte, string URL, io.Reader,
	// whatever fits the modality. Required.
	//
	// Wire form: bytes serialize as a base64 string under a
	// "binary" tag; strings stay as-is under a "text" tag. Other
	// Go types (io.Reader, structs) are not serializable and produce
	// an error when marshaled.
	Data any `json:"-"`

	// Metadata carries free-form annotations.
	Metadata map[string]any `json:"metadata,omitzero"`
}

// mediaWire is the on-the-wire shape of [Media]. It splits Data into
// (DataEncoding, DataValue) so JSON can carry the discriminator the Go
// type system loses when round-tripping through `any`.
type mediaWire struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	MimeType *mime.MIME     `json:"mime_type,omitempty"`
	Encoding string         `json:"data_encoding,omitempty"` // "bytes" | "text"
	Data     string         `json:"data,omitempty"`
	Metadata map[string]any `json:"metadata,omitzero"`
}

// MarshalJSON encodes Media with an explicit data-encoding tag so the
// caller's []byte vs string distinction round-trips. Returns an error
// for non-serializable Data (io.Reader, structs, ...).
func (m *Media) MarshalJSON() ([]byte, error) {
	out := mediaWire{
		ID:       m.ID,
		Name:     m.Name,
		MimeType: m.MimeType,
		Metadata: m.Metadata,
	}
	switch d := m.Data.(type) {
	case nil:
		// no data payload
	case []byte:
		out.Encoding = "bytes"
		out.Data = base64.StdEncoding.EncodeToString(d)
	case string:
		out.Encoding = "text"
		out.Data = d
	default:
		return nil, fmt.Errorf("media.MarshalJSON: unsupported Data type %T (only []byte and string round-trip)", m.Data)
	}
	return json.Marshal(out)
}

// UnmarshalJSON decodes Media, restoring Data as either []byte or
// string based on the data_encoding discriminator.
func (m *Media) UnmarshalJSON(data []byte) error {
	var in mediaWire
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	m.ID = in.ID
	m.Name = in.Name
	m.MimeType = in.MimeType
	m.Metadata = in.Metadata
	switch in.Encoding {
	case "":
		m.Data = nil
	case "bytes":
		decoded, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			return fmt.Errorf("media.UnmarshalJSON: data_encoding=bytes but body is not valid base64: %w", err)
		}
		m.Data = decoded
	case "text":
		m.Data = in.Data
	default:
		return fmt.Errorf("media.UnmarshalJSON: unknown data_encoding %q", in.Encoding)
	}
	return nil
}

// Both mimeType and data are required.
//
// Example:
//
//	mt, _ := mime.Parse("audio/mpeg")
//	m, err := media.NewMedia(mt, audioBytes)
func NewMedia(mimeType *mime.MIME, data any) (*Media, error) {
	if mimeType == nil {
		return nil, errors.New("media.NewMedia: mimeType must not be nil")
	}
	if data == nil {
		return nil, errors.New("media.NewMedia: data must not be nil")
	}

	return &Media{
		MimeType: mimeType,
		Data:     data,
		Metadata: make(map[string]any),
	}, nil
}

// Returns an error when Data is nil or holds a non-bytes value.
func (m *Media) DataAsBytes() ([]byte, error) {
	if m == nil || m.Data == nil {
		return nil, errors.New("media.DataAsBytes: data is nil")
	}

	data, ok := m.Data.([]byte)
	if !ok {
		return nil, fmt.Errorf("media.DataAsBytes: expected []byte, got %T", m.Data)
	}
	return data, nil
}

// Returns an error when
// Data is nil or holds a non-string value.
func (m *Media) DataAsString() (string, error) {
	if m == nil || m.Data == nil {
		return "", errors.New("media.DataAsString: data is nil")
	}

	data, ok := m.Data.(string)
	if !ok {
		return "", fmt.Errorf("media.DataAsString: expected string, got %T", m.Data)
	}
	return data, nil
}
