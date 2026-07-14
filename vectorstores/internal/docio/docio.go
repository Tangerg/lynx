package docio

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// EnsureID returns id unchanged when it's non-empty; otherwise it
// mints a fresh UUID. Used by every Add path's per-document loop.
func EnsureID(id string) string {
	if id != "" {
		return id
	}
	return uuid.NewString()
}

// MetadataOrEmpty returns m unchanged when non-nil; otherwise it
// returns an empty map so the document always carries the metadata
// field. Backends that round-trip JSON benefit from the consistent
// shape.
func MetadataOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// MarshalMetadata serializes m to JSON. A nil m maps to JSON `null`
// when nullSentinel is "null" (the pgvector / postgres default),
// otherwise to the supplied empty-object sentinel — most JSON column
// types accept "{}" too.
func MarshalMetadata(m map[string]any, nullSentinel string) ([]byte, error) {
	if m == nil {
		if nullSentinel == "" {
			nullSentinel = "null"
		}
		return []byte(nullSentinel), nil
	}
	return json.Marshal(m)
}

// UnmarshalMetadata reverses [MarshalMetadata]. Empty / null inputs
// produce a nil map.
func UnmarshalMetadata(b []byte) (map[string]any, error) {
	if len(b) == 0 || string(b) == "null" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FormatVectorLiteral renders a float32 slice as the textual
// "[v1,v2,...]" form accepted by pgvector / MariaDB / Oracle / TiDB
// / Cassandra. The values use the shortest round-trippable form.
func FormatVectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
