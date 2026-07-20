package toolloop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ToolResolver resolves the executable tool advertised by a model request.
// Resolve must return a non-nil Tool whenever ok is true. Resolver panics are
// returned as runner errors attributed to the requested tool name.
type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

var _ ToolResolver = (*tools.Registry)(nil)

func (s *runnerState) validateInput() error {
	if s == nil || s.request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidInput)
	}
	if err := s.request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidInput, err)
	}
	if len(s.request.Tools) == 0 {
		return nil
	}
	if valueIsNil(s.resolver) {
		return fmt.Errorf("%w: request advertises tools but resolver is nil", ErrInvalidInput)
	}
	for _, definition := range s.request.Tools {
		tool, ok, err := resolveTool(s.resolver, definition.Name)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		if !ok || valueIsNil(tool) {
			return fmt.Errorf("%w: advertised tool %q is not executable", ErrInvalidInput, definition.Name)
		}
		executableDefinition, err := toolDefinition(tool, definition.Name)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
		if !sameToolDefinition(definition, executableDefinition) {
			return fmt.Errorf("%w: advertised tool %q definition does not match executable tool", ErrInvalidInput, definition.Name)
		}
	}
	return nil
}

func resolveTool(resolver ToolResolver, name string) (tool tools.Tool, ok bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool resolver %T Resolve(%q) panicked", resolver, name), recovered)
		}
	}()
	tool, ok = resolver.Resolve(name)
	return tool, ok, nil
}

func toolDefinition(tool tools.Tool, name string) (definition chat.ToolDefinition, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool %q Definition panicked", name), recovered)
		}
	}()
	return tool.Definition(), nil
}

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
