package mcpserver

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	// MaxRemoteToolsPerServer bounds one complete tools/list projection from a
	// single connected MCP server.
	MaxRemoteToolsPerServer = 2_048

	// MaxRemoteToolDescriptionBytes bounds model-visible prose attached to one
	// remote tool descriptor.
	MaxRemoteToolDescriptionBytes = 64 << 10

	// MaxRemoteToolInputSchemaBytes bounds the encoded JSON Schema attached to
	// one remote tool descriptor.
	MaxRemoteToolInputSchemaBytes = 1 << 20
)

// ErrInvalidRemoteToolCatalog reports remote descriptor material that cannot
// be admitted to Runtime's complete management and model-facing catalogs.
var ErrInvalidRemoteToolCatalog = errors.New("mcp: invalid remote tool catalog")

// ValidateRemoteToolCount enforces the per-server complete-list bound.
func ValidateRemoteToolCount(count int) error {
	if count < 0 || count > MaxRemoteToolsPerServer {
		return fmt.Errorf(
			"%w: %d tools exceeds %d",
			ErrInvalidRemoteToolCatalog,
			count,
			MaxRemoteToolsPerServer,
		)
	}
	return nil
}

// ValidateRemoteToolDescription enforces the encoded remote-description
// envelope before the text enters either catalog.
func ValidateRemoteToolDescription(description string) error {
	if !utf8.ValidString(description) {
		return fmt.Errorf("%w: description is not valid UTF-8", ErrInvalidRemoteToolCatalog)
	}
	if len(description) > MaxRemoteToolDescriptionBytes {
		return fmt.Errorf(
			"%w: description uses %d bytes, maximum %d",
			ErrInvalidRemoteToolCatalog,
			len(description),
			MaxRemoteToolDescriptionBytes,
		)
	}
	return nil
}

// ValidateRemoteToolMaterial validates the complete model-visible material for
// one already-encoded remote descriptor.
func ValidateRemoteToolMaterial(description string, inputSchema []byte) error {
	if err := ValidateRemoteToolDescription(description); err != nil {
		return err
	}
	if _, err := ParseInputSchema(inputSchema); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRemoteToolCatalog, err)
	}
	return nil
}
