package mime

import (
	"fmt"
	"github.com/Tangerg/lynx/pkg/kv"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
	"github.com/bits-and-blooms/bitset"
	"strings"
)

const (
	wildcardType = "*"
	paramCharset = "charset"
)

var tokenBitSet *bitset.BitSet

func init() {
	ctl := bitset.New(128)
	for i := 0; i < 31; i++ {
		ctl.Set(uint(i))
	}
	ctl.Set(127)
	separatorChars := []rune{
		'(', ')', '<', '>', '@', ',', ';', ':', '\\', '"',
		'/', '[', ']', '?', '=', '{', '}', ' ', '\t',
	}
	separators := bitset.New(128)
	for _, char := range separatorChars {
		separators.Set(uint(char))
	}
	tokenBitSet = bitset.New(128)
	for i := uint(0); i < 128; i++ {
		tokenBitSet.Set(i)
	}
	tokenBitSet.InPlaceSymmetricDifference(ctl)
	tokenBitSet.InPlaceSymmetricDifference(separators)
}

type Mime struct {
	_type       string
	subType     string
	charset     string
	params      kv.KV[string, string]
	stringValue string
}

func (m *Mime) checkToken(token string) error {
	for _, char := range token {
		if !tokenBitSet.Test(uint(char)) {
			return fmt.Errorf("invalid character %s in token: %s", string(char), token)
		}
	}
	return nil
}

func (m *Mime) checkParam(k string, v string) error {
	err := m.checkToken(k)
	if err != nil {
		return err
	}
	if k == paramCharset {
		if m.charset == "" {
			m.charset = pkgStrings.UnQuote(v)
		}
		return nil
	}
	if !pkgStrings.IsQuoted(v) {
		return m.checkToken(v)
	}
	return nil
}

func (m *Mime) checkParams() error {
	for k, v := range m.params {
		err := m.checkParam(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

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

func (m *Mime) Type() string {
	return m._type
}
func (m *Mime) SubType() string {
	return m.subType
}
func (m *Mime) TypeAndSubType() string {
	return m._type + "/" + m.subType
}
func (m *Mime) Charset() string {
	return m.charset
}
func (m *Mime) Param(key string) (string, bool) {
	return m.params.Get(key)
}
func (m *Mime) Params() map[string]string {
	return m.params
}
func (m *Mime) String() string {
	if m.stringValue == "" {
		m.formatStringValue()
	}
	return m.stringValue
}
func (m *Mime) IsWildcardType() bool {
	return m._type == wildcardType
}
func (m *Mime) IsWildcardSubType() bool {
	return m.subType == wildcardType ||
		strings.HasPrefix(m.subType, "*+")
}
func (m *Mime) IsConcrete() bool {
	return !m.IsWildcardType() && !m.IsWildcardSubType()
}
func (m *Mime) GetSubtypeSuffix() string {
	suffixIndex := strings.LastIndexByte(m.subType, '+')
	if suffixIndex != -1 && len(m.subType) > suffixIndex {
		return m.subType[suffixIndex+1:]
	}
	return ""
}
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
func (m *Mime) IsCompatibleWith(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.Includes(other) || other.Includes(m)
}
func (m *Mime) EqualsType(other *Mime) bool {
	if other == nil {
		return false
	}
	return m._type == other._type
}
func (m *Mime) EqualsSubtype(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.subType == other.subType
}
func (m *Mime) EqualsTypeAndSubtype(other *Mime) bool {
	return m.EqualsType(other) &&
		m.EqualsSubtype(other)
}
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
func (m *Mime) EqualsCharset(other *Mime) bool {
	if other == nil {
		return false
	}
	return m.charset == other.charset
}
func (m *Mime) Equals(other *Mime) bool {
	return m.EqualsTypeAndSubtype(other) &&
		m.EqualsCharset(other) &&
		m.EqualsParams(other)
}
func (m *Mime) IsPresentIn(mimes []*Mime) bool {
	for _, mime := range mimes {
		if mime.EqualsTypeAndSubtype(m) {
			return true
		}
	}
	return false
}
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
func (m *Mime) IsLessSpecific(other *Mime) bool {
	return !m.IsMoreSpecific(other)
}
