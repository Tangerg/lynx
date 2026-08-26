package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const commandIDPrefix = "cli_"

// CommandID is the stable identity of one mutation intent. A caller creates it
// once and retains it across retries; adapters map it onto their transport's
// idempotency mechanism without inventing a second identity.
type CommandID string

func NewCommandID() (CommandID, error) {
	return newCommandID(rand.Reader)
}

func newCommandID(random io.Reader) (CommandID, error) {
	if random == nil {
		return "", errors.New("command identity source is nil")
	}
	var entropy [16]byte
	if _, err := io.ReadFull(random, entropy[:]); err != nil {
		return "", fmt.Errorf("generate command identity: %w", err)
	}
	return CommandID(commandIDPrefix + hex.EncodeToString(entropy[:])), nil
}

func (c CommandID) Validate() error {
	value := string(c)
	encoded, ok := strings.CutPrefix(value, commandIDPrefix)
	if !ok || len(encoded) != 32 {
		return errors.New("command id has an invalid shape")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return errors.New("command id has invalid entropy")
	}
	return nil
}
