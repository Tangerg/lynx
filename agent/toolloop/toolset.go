package toolloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

func toolsetDigest(definitions []chat.ToolDefinition) (string, error) {
	values := slices.Clone(definitions)
	slices.SortFunc(values, func(a, b chat.ToolDefinition) int { return bytes.Compare([]byte(a.Name), []byte(b.Name)) })
	hash := sha256.New()
	for i := range values {
		if err := values[i].Validate(); err != nil {
			return "", err
		}
		data, err := json.Marshal(values[i])
		if err != nil {
			return "", err
		}
		var normalized any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&normalized); err != nil {
			return "", err
		}
		canonical, err := json.Marshal(normalized)
		if err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(hash, "%d:", len(canonical)); err != nil {
			return "", err
		}
		_, _ = hash.Write(canonical)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameToolDefinition(a, b chat.ToolDefinition) bool {
	left, err := toolsetDigest([]chat.ToolDefinition{a})
	if err != nil {
		return false
	}
	right, err := toolsetDigest([]chat.ToolDefinition{b})
	return err == nil && left == right
}
