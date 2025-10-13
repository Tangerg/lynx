// Package mime provides functionality for handling MIME (Multipurpose Internet Mail Extensions) types.
// MIME types are standardized identifiers that represent the format and nature of files, documents,
// or byte streams, widely used in HTTP, email, and other internet protocols.
package mime

import (
	"strings"

	"github.com/Tangerg/lynx/pkg/maps"
)

const (
	// wildcardType represents the wildcard character used in MIME type patterns.
	// Example:
	// - In "*", the primary type is a wildcard that matches any type.
	// - In "video/*", the subType is a wildcard that matches any video type.
	wildcardType = "*"

	// paramCharset is the standard parameter name for character set specification.
	// Example: In "text/html; charset=UTF-8", "charset" is the parameter name.
	paramCharset = "charset"
)

// MIME represents a MIME type structure with its components and properties.
// It stores the type (e.g., "text"), subtype (e.g., "html"), optional charset,
// and additional parameters.
//
// Examples:
// - "text/html; charset=UTF-8" -> type="text", subType="html", charset="UTF-8"
// - "application/json" -> type="application", subType="json"
// - "image/svg+xml" -> type="image", subType="svg+xml"
type MIME struct {
	_type       string                       // The primary type component (e.g., "text", "application")
	subType     string                       // The subtype component (e.g., "html", "json")
	charset     string                       // The character set value (e.g., "UTF-8")
	params      maps.HashMap[string, string] // Additional parameters as key-value pairs
	stringCache string                       // Cached string representation for performance
}

func (m *MIME) MarshalJSON() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return []byte(m.String()), nil
}

func (m *MIME) UnmarshalJSON(data []byte) error {
	parsed, err := Parse(string(data))
	if err != nil {
		return err
	}

	m._type = parsed._type
	m.subType = parsed.subType
	m.charset = parsed.charset
	m.params = parsed.params
	m.stringCache = parsed.stringCache

	return nil
}

// formatStringValue constructs and caches the full string representation of the MIME type
// in the standard format "type/subtype; param1=value1; param2=value2".
//
// Example:
// For a MIME with type="text", subType="html", and params={"charset":"UTF-8"},
// the resulting string would be "text/html;charset=UTF-8"
func (m *MIME) formatStringValue() {
	stringBuilder := strings.Builder{}

	// Build the basic type/subtype structure
	stringBuilder.WriteString(m._type)
	stringBuilder.WriteString("/")
	stringBuilder.WriteString(m.subType)

	// Append all parameters
	m.params.ForEach(func(paramKey, paramValue string) {
		stringBuilder.WriteString(";")
		stringBuilder.WriteString(paramKey)
		stringBuilder.WriteString("=")
		stringBuilder.WriteString(paramValue)
	})

	m.stringCache = stringBuilder.String()
}

// Type returns the primary type component of this MIME type.
// Example: For "text/html", returns "text".
func (m *MIME) Type() string {
	return m._type
}

// SubType returns the subtype component of this MIME type.
// Example: For "application/json", returns "json".
func (m *MIME) SubType() string {
	return m.subType
}

// TypeAndSubType returns the combined type and subtype in the format "type/subtype".
// Example: For a MIME with type="image" and subType="png", returns "image/png".
func (m *MIME) TypeAndSubType() string {
	return m._type + "/" + m.subType
}

// FullType is an alias for TypeAndSubType
func (m *MIME) FullType() string {
	return m.TypeAndSubType()
}

// Charset returns the character set parameter value if specified.
// Example: For "text/html; charset=UTF-8", returns "UTF-8".
func (m *MIME) Charset() string {
	return m.charset
}

// Param retrieves a specific parameter value by key.
// Returns the value and a boolean indicating if the parameter exists.
//
// Example:
// For "application/json; version=1.0":
// m.Param("version") returns "1.0", true
// m.Param("charset") returns "", false
func (m *MIME) Param(paramKey string) (string, bool) {
	return m.params.Get(paramKey)
}

// Params returns all parameters as a map of key-value pairs.
// Example: For "text/html; charset=UTF-8; level=1", returns {"charset":"UTF-8", "level":"1"}
func (m *MIME) Params() map[string]string {
	return m.params
}

// String returns the complete string representation of this MIME type.
// Uses the cached value if available for better performance.
//
// Example outputs:
// - "text/html;charset=UTF-8"
// - "application/json"
// - "image/png;quality=high"
func (m *MIME) String() string {
	if m.stringCache == "" {
		m.formatStringValue()
	}
	return m.stringCache
}

// IsWildcardType checks if this MIME type has a wildcard primary type.
// Example: For "*/html", returns true; for "text/html", returns false.
func (m *MIME) IsWildcardType() bool {
	return m._type == wildcardType
}

// IsWildcardSubType checks if this MIME type has a wildcard subtype.
// Returns true for both "*" and "*+suffix" patterns.
//
// Examples:
// - "text/*" -> true
// - "application/*+json" -> true
// - "text/html" -> false
func (m *MIME) IsWildcardSubType() bool {
	return m.subType == wildcardType || strings.HasPrefix(m.subType, "*+")
}

// IsConcrete returns true if neither the type nor subtype contains wildcards.
// A concrete MIME type represents a specific format rather than a pattern.
//
// Examples:
// - "text/html" -> true (concrete)
// - "text/*" -> false (has wildcard)
// - "*/json" -> false (has wildcard)
func (m *MIME) IsConcrete() bool {
	return !m.IsWildcardType() && !m.IsWildcardSubType()
}

// GetSubtypeSuffix extracts the suffix part after the '+' character in the subtype.
//
// Examples:
// - "application/vnd.api+json" -> "json"
// - "application/xml+xhtml" -> "xhtml"
// - "text/html" -> "" (no suffix)
func (m *MIME) GetSubtypeSuffix() string {
	plusIndex := strings.LastIndexByte(m.subType, '+')
	if plusIndex != -1 && len(m.subType) > plusIndex {
		return m.subType[plusIndex+1:]
	}
	return ""
}

// Includes checks if this MIME type includes (is more general than) another MIME type.
// Implements the MIME type matching rules for wildcard patterns.
//
// Examples:
// - "text/*" includes "text/html" (wildcard subtype match)
// - "*/*" includes "image/png" (wildcard type and subtype)
// - "application/*+json" includes "application/vnd.api+json" (suffix match)
// - "text/html" does not include "text/plain" (different concrete subtypes)
func (m *MIME) Includes(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}

	// Wildcard type includes all types
	if m.IsWildcardType() {
		return true
	}

	// Type must match if not wildcard
	if !m.EqualsType(otherMime) {
		return false
	}

	// Exact subtype match
	if m.EqualsSubtype(otherMime) {
		return true
	}

	// Non-wildcard subtype doesn't include others
	if !m.IsWildcardSubType() {
		return false
	}

	// Handle wildcard subtype with suffix matching
	currentPlusIndex := strings.LastIndexByte(m.subType, '+')
	if currentPlusIndex == -1 {
		return true
	}

	otherPlusIndex := strings.LastIndexByte(otherMime.subType, '+')
	if otherPlusIndex == -1 {
		return false
	}

	currentSubtypePrefix := m.subType[0:currentPlusIndex]
	currentSubtypeSuffix := m.subType[currentPlusIndex+1:]
	otherSubtypeSuffix := otherMime.subType[otherPlusIndex+1:]

	return currentSubtypeSuffix == otherSubtypeSuffix && currentSubtypePrefix == wildcardType
}

// IsCompatibleWith checks if this MIME type is compatible with another MIME type.
// Two MIME types are compatible if either one includes the other.
//
// Examples:
// - "text/*" is compatible with "text/html" (one includes the other)
// - "application/json" is compatible with "application/json" (they're equal)
// - "text/html" is not compatible with "application/json" (different types)
func (m *MIME) IsCompatibleWith(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.Includes(otherMime) || otherMime.Includes(m)
}

// EqualsType checks if this MIME type's primary type equals another's.
//
// Examples:
// - "text/html" and "text/plain" have equal types ("text")
// - "image/png" and "application/json" have different types
func (m *MIME) EqualsType(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m._type == otherMime._type
}

// EqualsSubtype checks if this MIME type's subtype equals another's.
//
// Examples:
// - "text/html" and "application/html" have equal subtypes ("html")
// - "text/plain" and "text/html" have different subtypes
func (m *MIME) EqualsSubtype(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.subType == otherMime.subType
}

// EqualsTypeAndSubtype checks if both the type and subtype match another MIME type.
// Only compares the type/subtype components, ignoring parameters.
//
// Examples:
// - "text/html; charset=UTF-8" equals "text/html" (parameters ignored)
// - "text/html" does not equal "text/plain" (different subtypes)
func (m *MIME) EqualsTypeAndSubtype(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.EqualsType(otherMime) && m.EqualsSubtype(otherMime)
}

// EqualsParams checks if all parameters match another MIME type's parameters.
// Requires the same parameters with identical values in both MIME types.
//
// Examples:
// - "text/html; charset=UTF-8" and "text/plain; charset=UTF-8" have equal params
// - "text/html; charset=UTF-8" and "text/html; charset=ASCII" have different params
// - "text/html; charset=UTF-8" and "text/html" have different params (one has none)
func (m *MIME) EqualsParams(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}

	// Check if parameter counts match
	if m.params.Size() != otherMime.params.Size() {
		return false
	}

	// Compare each parameter
	parametersEqual := true
	m.params.ForEach(func(paramKey, paramValue string) {
		otherValue, ok := otherMime.params.Get(paramKey)
		if ok {
			if paramValue != otherValue {
				parametersEqual = false
			}
		}
	})

	return parametersEqual
}

// EqualsCharset checks if this MIME type's charset equals another's.
//
// Examples:
// - "text/html; charset=UTF-8" and "text/plain; charset=UTF-8" have equal charsets
// - "text/html; charset=UTF-8" and "text/html; charset=ASCII" have different charsets
func (m *MIME) EqualsCharset(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.charset == otherMime.charset
}

// Equals performs a complete comparison of all MIME type components.
// Returns true only if type, subtype, charset, and all parameters match exactly.
//
// Examples:
// - "text/html; charset=UTF-8" equals "text/html; charset=UTF-8"
// - "text/html; charset=UTF-8" does not equal "text/html; charset=UTF-8; level=1"
// - "text/html; charset=UTF-8" does not equal "text/plain; charset=UTF-8"
func (m *MIME) Equals(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.EqualsTypeAndSubtype(otherMime) &&
		m.EqualsCharset(otherMime) &&
		m.EqualsParams(otherMime)
}

// IsPresentIn checks if this MIME type is present in a list of MIME types.
// Matches based on type and subtype only, ignoring parameters.
//
// Example:
// For a list ["text/html", "application/json"], "text/html; charset=UTF-8" returns true
func (m *MIME) IsPresentIn(mimeList []*MIME) bool {
	for _, mimeType := range mimeList {
		if mimeType.EqualsTypeAndSubtype(m) {
			return true
		}
	}
	return false
}

// IsMoreSpecific checks if this MIME type is more specific than another.
// A MIME type is more specific if it has concrete types where the other has wildcards,
// or if it has more parameters when types are equal.
//
// Examples:
// - "text/html" is more specific than "text/*" (concrete vs wildcard)
// - "text/html; charset=UTF-8" is more specific than "text/html" (more parameters)
// - "text/html" is not more specific than "application/json" (different types)
func (m *MIME) IsMoreSpecific(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}

	// Check type specificity
	if m.IsWildcardType() && !otherMime.IsWildcardType() {
		return false
	}
	if !m.IsWildcardType() && otherMime.IsWildcardType() {
		return true
	}

	// Check subtype specificity
	if m.IsWildcardSubType() && !otherMime.IsWildcardSubType() {
		return false
	}
	if !m.IsWildcardSubType() && otherMime.IsWildcardSubType() {
		return true
	}

	// For equal types, compare parameter count
	if m.EqualsTypeAndSubtype(otherMime) {
		return m.params.Size() > otherMime.params.Size()
	}

	return false
}

// IsLessSpecific is the inverse of IsMoreSpecific.
// Returns true if this MIME type is less specific (more general) than the other.
//
// Examples:
// - "text/*" is less specific than "text/html" (wildcard vs concrete)
// - "text/html" is less specific than "text/html; charset=UTF-8" (fewer parameters)
func (m *MIME) IsLessSpecific(otherMime *MIME) bool {
	return !m.IsMoreSpecific(otherMime)
}

// Clone creates a deep copy of this MIME type.
// Returns a new instance with the same properties but separate memory allocation.
//
// Example:
// myMime = "text/html; charset=UTF-8"
// clone = myMime.Clone() // Creates a new instance with the same values
func (m *MIME) Clone() *MIME {
	clonedMime, _ := NewBuilder().
		FromMime(m).
		Build()
	return clonedMime
}
