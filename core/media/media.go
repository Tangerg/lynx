package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

var (
	// ErrNilMedia reports a nil Media receiver.
	ErrNilMedia = errors.New("media: nil media")
	// ErrInvalidMIME reports a missing or malformed MIME media type.
	ErrInvalidMIME = errors.New("media: invalid MIME type")
	// ErrInvalidSource reports a missing, ambiguous, or malformed source.
	ErrInvalidSource = errors.New("media: invalid source")
)

// SourceKind identifies which Source value is active.
type SourceKind string

const (
	// SourceBytes carries an inline byte payload.
	SourceBytes SourceKind = "bytes"
	// SourceURI carries an absolute URI resolved by a provider or client.
	SourceURI SourceKind = "uri"
	// SourceReference carries a provider-native media reference.
	SourceReference SourceKind = "reference"
)

// Source is a tagged union. Kind selects exactly one of Bytes, URI, or
// Ref; all other fields must be empty.
type Source struct {
	Kind  SourceKind `json:"kind"`
	Bytes []byte     `json:"bytes,omitempty"`
	URI   string     `json:"uri,omitempty"`
	Ref   string     `json:"ref,omitempty"`
}

// Validate verifies the tagged-source invariant.
func (s Source) Validate() error {
	switch s.Kind {
	case SourceBytes:
		if len(s.Bytes) == 0 || s.URI != "" || s.Ref != "" {
			return fmt.Errorf("%w: kind %q requires non-empty bytes and no URI or reference", ErrInvalidSource, s.Kind)
		}
	case SourceURI:
		if len(s.Bytes) != 0 || s.URI == "" || s.Ref != "" {
			return fmt.Errorf("%w: kind %q requires a URI and no bytes or reference", ErrInvalidSource, s.Kind)
		}
		parsed, err := url.Parse(s.URI)
		if err != nil || parsed.Scheme == "" || (parsed.Opaque == "" && parsed.Host == "" && parsed.Path == "") {
			return fmt.Errorf("%w: %q is not an absolute URI", ErrInvalidSource, s.URI)
		}
	case SourceReference:
		if len(s.Bytes) != 0 || s.URI != "" || strings.TrimSpace(s.Ref) == "" {
			return fmt.Errorf("%w: kind %q requires a reference and no bytes or URI", ErrInvalidSource, s.Kind)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidSource, s.Kind)
	}
	return nil
}

// Media describes a media payload without retaining runtime-only objects.
type Media struct {
	MIME     string       `json:"mime"`
	Source   Source       `json:"source"`
	ID       string       `json:"id,omitempty"`
	Name     string       `json:"name,omitempty"`
	Metadata metadata.Map `json:"metadata,omitempty"`
}

// Clone returns an independent copy of m. It is nil-safe.
func (m *Media) Clone() *Media {
	if m == nil {
		return nil
	}
	clone := *m
	clone.Source.Bytes = slices.Clone(m.Source.Bytes)
	clone.Metadata = m.Metadata.Clone()
	return &clone
}

// NewBytes returns Media containing an inline byte payload. The input is
// copied so later caller mutations cannot change the protocol value.
func NewBytes(mimeType string, data []byte) (*Media, error) {
	m := &Media{
		MIME: mimeType,
		Source: Source{
			Kind:  SourceBytes,
			Bytes: append([]byte(nil), data...),
		},
		Metadata: metadata.Map{},
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// NewURI returns Media referencing an absolute URI.
func NewURI(mimeType, uri string) (*Media, error) {
	m := &Media{
		MIME: mimeType,
		Source: Source{
			Kind: SourceURI,
			URI:  uri,
		},
		Metadata: metadata.Map{},
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// NewReference returns Media carrying a provider-native reference.
func NewReference(mimeType, ref string) (*Media, error) {
	m := &Media{
		MIME: mimeType,
		Source: Source{
			Kind: SourceReference,
			Ref:  ref,
		},
		Metadata: metadata.Map{},
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// Validate verifies MIME, source, and nested metadata invariants.
func (m *Media) Validate() error {
	if m == nil {
		return ErrNilMedia
	}
	mediaType, _, err := mime.ParseMediaType(m.MIME)
	if err != nil || !strings.Contains(mediaType, "/") {
		return fmt.Errorf("%w: %q", ErrInvalidMIME, m.MIME)
	}
	if err := m.Source.Validate(); err != nil {
		return err
	}
	if err := m.Metadata.Validate(); err != nil {
		return fmt.Errorf("media: metadata: %w", err)
	}
	return nil
}

// Bytes returns a copy of the inline byte payload.
func (m *Media) Bytes() ([]byte, error) {
	if m == nil {
		return nil, ErrNilMedia
	}
	if m.Source.Kind != SourceBytes {
		return nil, fmt.Errorf("%w: source kind is %q, not %q", ErrInvalidSource, m.Source.Kind, SourceBytes)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return append([]byte(nil), m.Source.Bytes...), nil
}

// URI returns the URI source.
func (m *Media) URI() (string, error) {
	if m == nil {
		return "", ErrNilMedia
	}
	if m.Source.Kind != SourceURI {
		return "", fmt.Errorf("%w: source kind is %q, not %q", ErrInvalidSource, m.Source.Kind, SourceURI)
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	return m.Source.URI, nil
}

// Reference returns the provider-native reference source.
func (m *Media) Reference() (string, error) {
	if m == nil {
		return "", ErrNilMedia
	}
	if m.Source.Kind != SourceReference {
		return "", fmt.Errorf("%w: source kind is %q, not %q", ErrInvalidSource, m.Source.Kind, SourceReference)
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	return m.Source.Ref, nil
}

// MarshalJSON validates Media before writing its wire representation. Byte
// sources use encoding/json's standard base64 representation for []byte.
func (m Media) MarshalJSON() ([]byte, error) {
	if err := (&m).Validate(); err != nil {
		return nil, err
	}
	type wireMedia Media
	return json.Marshal(wireMedia(m))
}

// UnmarshalJSON decodes and validates Media before replacing the receiver.
func (m *Media) UnmarshalJSON(data []byte) error {
	if m == nil {
		return ErrNilMedia
	}
	type wireMedia Media
	var decoded wireMedia
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("media: decode: %w", err)
	}
	candidate := Media(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}
