// Package mime provides functionality for handling MIME (Multipurpose Internet Mail Extensions) types.
package mime

import (
	"strings"

	"github.com/Tangerg/lynx/pkg/kv"
)

const (
	// wildcardType represents the wildcard character used in MIME type patterns
	wildcardType = "*"
	// paramCharset is the standard parameter name for character set specification
	paramCharset = "charset"
)

// Mime represents a MIME type structure with its components and properties.
// It stores the type (e.g., "text"), subtype (e.g., "html"), optional charset,
// and additional parameters.
type Mime struct {
	_type       string                // The primary type component (e.g., "text", "application")
	subType     string                // The subtype component (e.g., "html", "json")
	charset     string                // The character set value (e.g., "UTF-8")
	params      kv.KV[string, string] // Additional parameters as key-value pairs
	stringValue string                // Cached string representation for performance
}

// formatStringValue constructs and caches the full string representation of the MIME type
// in the standard format "type/subtype; param1=value1; param2=value2"
func (m *Mime) formatStringValue() {
	sb := strings.Builder{}
	sb.WriteString(m._type)
	sb.WriteString("/")
	sb.WriteString(m.subType)
	m.params.ForEach(func(k, v string) {
		sb.WriteString(";")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	})
	m.stringValue = sb.String()
}

// Type returns the primary type component of this MIME type
func (m *Mime) Type() string {
	return m._type
}

// SubType returns the subtype component of this MIME type
func (m *Mime) SubType() string {
	return m.subType
}

// TypeAndSubType returns the combined type and subtype in the format "type/subtype"
func (m *Mime) TypeAndSubType() string {
	return m._type + "/" + m.subType
}

// Charset returns the character set parameter value if specified
func (m *Mime) Charset() string {
	return m.charset
}

// Param retrieves a specific parameter value by key
// Returns the value and a boolean indicating if the parameter exists
func (m *Mime) Param(key string) (string, bool) {
	return m.params.Get(key)
}

// Params returns all parameters as a map of key-value pairs
func (m *Mime) Params() map[string]string {
	return m.params
}

// String returns the complete string representation of this MIME type
// Uses the cached value if available for better performance
func (m *Mime) String() string {
	if m.stringValue == "" {
		m.formatStringValue()
	}
	return m.stringValue
}

// IsWildcardType checks if this MIME type has a wildcard primary type (e.g., "*/html")
func (m *Mime) IsWildcardType() bool {
	return m._type == wildcardType
}

// IsWildcardSubType checks if this MIME type has a wildcard subtype
// Returns true for both "*" and "*+suffix" patterns (e.g., "text/*" or "application/*+json")
func (m *Mime) IsWildcardSubType() bool {
	return m.subType == wildcardType ||
		strings.HasPrefix(m.subType, "*+")
}

// IsConcrete returns true if neither the type nor subtype contains wildcards
// A concrete MIME type represents a specific format rather than a pattern
func (m *Mime) IsConcrete() bool {
	return !m.IsWildcardType() && !m.IsWildcardSubType()
}

// GetSubtypeSuffix extracts the suffix part after the '+' character in the subtype
// For example, in "application/vnd.api+json", the suffix is "json"
func (m *Mime) GetSubtypeSuffix() string {
	suffixIndex := strings.LastIndexByte(m.subType, '+')
	if suffixIndex != -1 && len(m.subType) > suffixIndex {
		return m.subType[suffixIndex+1:]
	}
	return ""
}

// Includes checks if this MIME type includes (is more general than) another MIME type
// Implements the MIME type matching rules for wildcard patterns
func (m *Mime) Includes(other *Mime) bool {
	if other == nil {
		return false
	}
	if m.IsWildcardType() {
		return true
	}
	if !m.EqualsType(other) {
		return false
	}
	if m.EqualsSubtype(other) {
		return true
	}
	if !m.IsWildcardSubType() {
		return false
	}
	thisPlusIdx := strings.LastIndexByte(m.subType, '+')
	if thisPlusIdx == -1 {
		return true
	}
	otherPlusIdx := strings.LastIndexByte(other.subType, '+')
	if otherPlusIdx == -1 {
		return false
	}
	thisSubtypeNoSuffix := m.subType[0:thisPlusIdx]
	thisSubtypeSuffix := m.subType[thisPlusIdx+1:]
	otherSubtypeSuffix := m.subType[otherPlusIdx+1:]

	return thisSubtypeSuffix == otherSubtypeSuffix &&
		thisSubtypeNoSuffix == wildcardType
}

// IsCompatibleWith checks if this MIME type is compatible with another MIME type
// Two MIME types are compatible if either one includes the other
func (m *Mime) IsCompatibleWith(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.Includes(other) || other.Includes(m)
}

// EqualsType checks if this MIME type's primary type equals another's
func (m *Mime) EqualsType(other *Mime) bool {
	if other == nil {
		return false
	}
	return m._type == other._type
}

// EqualsSubtype checks if this MIME type's subtype equals another's
func (m *Mime) EqualsSubtype(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.subType == other.subType
}

// EqualsTypeAndSubtype checks if both the type and subtype match another MIME type
// Only compares the type/subtype components, ignoring parameters
func (m *Mime) EqualsTypeAndSubtype(other *Mime) bool {
	return m.EqualsType(other) &&
		m.EqualsSubtype(other)
}

// EqualsParams checks if all parameters match another MIME type's parameters
// Requires the same parameters with identical values in both MIME types
func (m *Mime) EqualsParams(other *Mime) bool {
	if other == nil {
		return false
	}
	if m.params.Size() != other.params.Size() {
		return false
	}
	var equal = true
	m.params.ForEach(func(k, v string) {
		if v != other.params.Value(k) {
			equal = false
		}
	})
	return equal
}

// EqualsCharset checks if this MIME type's charset equals another's
func (m *Mime) EqualsCharset(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.charset == other.charset
}

// Equals performs a complete comparison of all MIME type components
// Returns true only if type, subtype, charset, and all parameters match exactly
func (m *Mime) Equals(other *Mime) bool {
	return m.EqualsTypeAndSubtype(other) &&
		m.EqualsCharset(other) &&
		m.EqualsParams(other)
}

// IsPresentIn checks if this MIME type is present in a list of MIME types
// Matches based on type and subtype only, ignoring parameters
func (m *Mime) IsPresentIn(mimes []*Mime) bool {
	for _, mime := range mimes {
		if mime.EqualsTypeAndSubtype(m) {
			return true
		}
	}
	return false
}

// IsMoreSpecific checks if this MIME type is more specific than another
// A MIME type is more specific if it has concrete types where the other has wildcards,
// or if it has more parameters when types are equal
func (m *Mime) IsMoreSpecific(other *Mime) bool {
	if other == nil {
		return false
	}
	if m.IsWildcardType() && !other.IsWildcardType() {
		return false
	}
	if !m.IsWildcardType() && other.IsWildcardType() {
		return true
	}
	if m.IsWildcardSubType() && !other.IsWildcardSubType() {
		return false
	}
	if !m.IsWildcardSubType() && other.IsWildcardSubType() {
		return true
	}
	if m.EqualsTypeAndSubtype(other) {
		return m.params.Size() > other.params.Size()
	}
	return false
}

// IsLessSpecific is the inverse of IsMoreSpecific
// Returns true if this MIME type is less specific (more general) than the other
func (m *Mime) IsLessSpecific(other *Mime) bool {
	return !m.IsMoreSpecific(other)
}

// Clone creates a deep copy of this MIME type
// Returns a new instance with the same properties but separate memory allocation
func (m *Mime) Clone() *Mime {
	newM, _ := NewBuilder().
		FromMime(m).
		Build()
	return newM
}
