package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	treeIncarnationIDPrefix    = "incarnation:"
	treeIncarnationRandomBytes = 16
)

var ErrInvalidTreeIncarnationID = errors.New("agent: invalid tree incarnation identity")

// TreeIncarnationID identifies the one active writer generation of a durable
// Process tree. Its zero value is invalid.
type TreeIncarnationID struct {
	value string
}

// ParseTreeIncarnationID validates the canonical wire representation of a
// tree incarnation identity.
func ParseTreeIncarnationID(value string) (TreeIncarnationID, error) {
	encoded, ok := strings.CutPrefix(value, treeIncarnationIDPrefix)
	if !ok || len(encoded) != treeIncarnationRandomBytes*2 || encoded != strings.ToLower(encoded) {
		return TreeIncarnationID{}, ErrInvalidTreeIncarnationID
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return TreeIncarnationID{}, fmt.Errorf("%w: %w", ErrInvalidTreeIncarnationID, err)
	}
	return TreeIncarnationID{value: value}, nil
}

func newTreeIncarnationID() (TreeIncarnationID, error) {
	var random [treeIncarnationRandomBytes]byte
	if _, err := rand.Read(random[:]); err != nil {
		return TreeIncarnationID{}, fmt.Errorf("agent: generate TreeIncarnationID: %w", err)
	}
	return ParseTreeIncarnationID(treeIncarnationIDPrefix + hex.EncodeToString(random[:]))
}

func (i TreeIncarnationID) String() string { return i.value }

func (i TreeIncarnationID) Valid() bool {
	_, err := ParseTreeIncarnationID(i.value)
	return err == nil
}

func (i TreeIncarnationID) MarshalText() ([]byte, error) {
	if !i.Valid() {
		return nil, ErrInvalidTreeIncarnationID
	}
	return []byte(i.value), nil
}

func (i *TreeIncarnationID) UnmarshalText(text []byte) error {
	if i == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidTreeIncarnationID)
	}
	value, err := ParseTreeIncarnationID(string(text))
	if err != nil {
		return err
	}
	*i = value
	return nil
}
