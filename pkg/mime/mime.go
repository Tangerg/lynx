package mime

import (
	"encoding/json"
	"strings"

	"github.com/Tangerg/lynx/pkg/maps"
)

const (
	// wildcardType is the "*" character used for wildcard type or subtype.
	wildcardType = "*"
	// paramCharset is the standard parameter name for the character set.
	paramCharset = "charset"
)

// MIME represents a parsed MIME type with its primary type, subtype,
// optional charset, and additional parameters. Construct values with
// [New], [Parse], or [Builder]; instances are immutable once built.
type MIME struct {
	_type        string
	subType      string
	charset      string
	params       maps.HashMap[string, string]
	cachedString string
}

// MarshalJSON encodes m as its canonical string form, JSON-quoted.
func (m *MIME) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m.String())
}

// UnmarshalJSON decodes a JSON-quoted MIME type string into m.
func (m *MIME) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}

	m._type = parsed._type
	m.subType = parsed.subType
	m.charset = parsed.charset
	m.params = parsed.params
	m.cachedString = parsed.cachedString

	return nil
}

// formatStringValue builds and caches the "type/subtype;k=v" form.
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

	m.cachedString = stringBuilder.String()
}

// Type returns the primary type, e.g. "text" for "text/html".
func (m *MIME) Type() string {
	return m._type
}

// SubType returns the subtype, e.g. "html" for "text/html".
func (m *MIME) SubType() string {
	return m.subType
}

// TypeAndSubType returns "type/subtype" without parameters.
func (m *MIME) TypeAndSubType() string {
	return m._type + "/" + m.subType
}

// FullType is an alias for [MIME.TypeAndSubType].
func (m *MIME) FullType() string {
	return m.TypeAndSubType()
}

// Charset returns the charset parameter value, or "" if unset.
func (m *MIME) Charset() string {
	return m.charset
}

// Param returns the value of the named parameter and whether it is set.
func (m *MIME) Param(paramKey string) (string, bool) {
	return m.params.Get(paramKey)
}

// Params returns the parameter map. The returned map is owned by m and
// must not be modified by the caller.
func (m *MIME) Params() map[string]string {
	return m.params
}

// String returns the canonical "type/subtype;k=v" form. The result is
// cached on first call. A nil receiver returns "".
func (m *MIME) String() string {
	if m == nil {
		return ""
	}
	if m.cachedString == "" {
		m.formatStringValue()
	}
	return m.cachedString
}

// IsWildcardType reports whether the primary type is "*".
func (m *MIME) IsWildcardType() bool {
	return m._type == wildcardType
}

// IsWildcardSubType reports whether the subtype is "*" or "*+suffix".
func (m *MIME) IsWildcardSubType() bool {
	return m.subType == wildcardType || strings.HasPrefix(m.subType, "*+")
}

// IsConcrete reports whether neither the type nor the subtype is a
// wildcard.
func (m *MIME) IsConcrete() bool {
	return !m.IsWildcardType() && !m.IsWildcardSubType()
}

// GetSubtypeSuffix returns the part after the last '+' in the subtype,
// or "" if none. For "application/vnd.api+json" it returns "json".
func (m *MIME) GetSubtypeSuffix() string {
	plusIndex := strings.LastIndexByte(m.subType, '+')
	if plusIndex != -1 && len(m.subType) > plusIndex {
		return m.subType[plusIndex+1:]
	}
	return ""
}

// Includes reports whether m is a superset of otherMime under the
// MIME wildcard rules. For example, "text/*" includes "text/html" and
// "application/*+json" includes "application/vnd.api+json".
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

// IsCompatibleWith reports whether m and otherMime include each other
// in either direction.
func (m *MIME) IsCompatibleWith(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.Includes(otherMime) || otherMime.Includes(m)
}

// EqualsType reports whether the primary types match.
func (m *MIME) EqualsType(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m._type == otherMime._type
}

// EqualsSubtype reports whether the subtypes match.
func (m *MIME) EqualsSubtype(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.subType == otherMime.subType
}

// EqualsTypeAndSubtype reports whether both type and subtype match,
// ignoring parameters.
func (m *MIME) EqualsTypeAndSubtype(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.EqualsType(otherMime) && m.EqualsSubtype(otherMime)
}

// EqualsParams reports whether the parameter maps are identical.
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
		} else {
			parametersEqual = false
		}
	})

	return parametersEqual
}

// EqualsCharset reports whether the charset values match.
func (m *MIME) EqualsCharset(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.charset == otherMime.charset
}

// Equals reports whether m and otherMime have the same type, subtype,
// charset, and parameters.
func (m *MIME) Equals(otherMime *MIME) bool {
	if otherMime == nil {
		return false
	}
	return m.EqualsTypeAndSubtype(otherMime) &&
		m.EqualsCharset(otherMime) &&
		m.EqualsParams(otherMime)
}

// IsPresentIn reports whether mimeList contains a type whose primary
// type and subtype match m. Parameters are ignored.
func (m *MIME) IsPresentIn(mimeList []*MIME) bool {
	for _, mimeType := range mimeList {
		if mimeType.EqualsTypeAndSubtype(m) {
			return true
		}
	}
	return false
}

// IsMoreSpecific reports whether m is strictly more specific than
// otherMime: concrete components beat wildcards, and for equal
// type/subtype the value with more parameters wins.
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

// IsLessSpecific is the inverse of [MIME.IsMoreSpecific].
func (m *MIME) IsLessSpecific(otherMime *MIME) bool {
	return !m.IsMoreSpecific(otherMime)
}

// Clone returns a deep copy of m with its own parameter map.
func (m *MIME) Clone() *MIME {
	clonedMime, _ := NewBuilder().
		FromMime(m).
		Build()
	return clonedMime
}
