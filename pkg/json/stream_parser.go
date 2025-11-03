package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode"
)

// StreamParserConfig holds configuration for the stream parser.
type StreamParserConfig struct {
	// BufferSize specifies the size of read buffer (default: 4096)
	BufferSize int

	// Reader is the source to read JSON stream from
	Reader io.Reader

	// OnArray is called when a complete top-level array is parsed
	OnArray func([]any) error

	// OnObject is called when a complete top-level object is parsed
	OnObject func(map[string]any) error

	// OnValue is called when a complete top-level primitive value is parsed
	// (string, number, boolean, null)
	OnValue func(any) error

	// OnError is called when parsing errors occur
	OnError func(error)
}

// validate checks if the configuration is valid.
func (c *StreamParserConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Reader == nil {
		return errors.New("reader must not be nil")
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 4096
	}
	return nil
}

// StreamParser parses JSON stream incrementally.
type StreamParser struct {
	bufferSize  int
	scopes      []string
	reader      io.Reader
	buffers     []*bytes.Buffer
	topLevelBuf *bytes.Buffer
	inString    bool
	escaped     bool
	charCount   int
	onArray     func([]any) error
	onObject    func(map[string]any) error
	onValue     func(any) error
	onError     func(error)
}

// NewStreamParser creates a new stream parser with the given configuration.
func NewStreamParser(config *StreamParserConfig) (*StreamParser, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &StreamParser{
		bufferSize:  config.BufferSize,
		scopes:      make([]string, 0, 8),
		reader:      config.Reader,
		buffers:     make([]*bytes.Buffer, 0, 8),
		topLevelBuf: new(bytes.Buffer),
		onArray:     config.OnArray,
		onObject:    config.OnObject,
		onValue:     config.OnValue,
		onError:     config.OnError,
	}, nil
}

// Parse reads and parses the JSON stream.
func (p *StreamParser) Parse() error {
	buf := make([]byte, p.bufferSize)

	for {
		n, err := p.reader.Read(buf)
		if n > 0 {
			if err = p.processBytes(buf[:n]); err != nil {
				return fmt.Errorf("process bytes: %w", err)
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				// Process any remaining top-level value
				if err = p.flushTopLevel(); err != nil {
					return err
				}
				// Ensure all scopes are closed
				if len(p.scopes) > 0 {
					return fmt.Errorf("unexpected EOF: unclosed %s", p.scopes[len(p.scopes)-1])
				}
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}
	}
}

// processBytes processes a chunk of bytes.
func (p *StreamParser) processBytes(data []byte) error {
	for _, char := range data {
		if err := p.processChar(char); err != nil {
			return err
		}
	}
	return nil
}

// processChar processes a single character.
func (p *StreamParser) processChar(char byte) error {
	p.charCount++

	// Handle string state
	if char == '"' && !p.escaped {
		p.inString = !p.inString
		p.readChar(char)
		return nil
	}

	// Handle escape sequences
	if p.inString {
		p.readChar(char)
		if p.escaped {
			p.escaped = false
		} else if char == '\\' {
			p.escaped = true
		}
		return nil
	}

	// Process structural characters
	switch char {
	case '{':
		p.startScope("object")
		p.readChar(char)
	case '}':
		p.readChar(char)
		if err := p.endScope("object"); err != nil {
			return err
		}
	case '[':
		p.startScope("array")
		p.readChar(char)
	case ']':
		p.readChar(char)
		if err := p.endScope("array"); err != nil {
			return err
		}
	case ',':
		// At top level, comma separates multiple JSON values
		if len(p.scopes) == 0 {
			if err := p.flushTopLevel(); err != nil {
				return err
			}
		} else {
			p.readChar(char)
		}
	default:
		// Skip whitespace
		if unicode.IsSpace(rune(char)) {
			// If at top level and we have content, might be end of a value
			if len(p.scopes) == 0 && p.topLevelBuf.Len() > 0 {
				// Check if this completes a primitive value
				if p.isCompletePrimitive() {
					if err := p.flushTopLevel(); err != nil {
						return err
					}
				}
			}
			// Don't write whitespace at top level
			if len(p.scopes) > 0 {
				p.readChar(char)
			}
		} else {
			p.readChar(char)
		}
	}

	return nil
}

// isCompletePrimitive checks if the top-level buffer contains a complete primitive value.
func (p *StreamParser) isCompletePrimitive() bool {
	data := bytes.TrimSpace(p.topLevelBuf.Bytes())
	if len(data) == 0 {
		return false
	}

	// Quick validation: try to unmarshal
	var v any
	return json.Unmarshal(data, &v) == nil
}

// flushTopLevel processes and dispatches the top-level buffer content.
func (p *StreamParser) flushTopLevel() error {
	data := bytes.TrimSpace(p.topLevelBuf.Bytes())
	if len(data) == 0 {
		return nil
	}

	defer p.topLevelBuf.Reset()

	// Try to parse as any JSON value
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("unmarshal top-level value at position %d: %w", p.charCount, err)
	}

	// Dispatch based on type
	if p.onValue != nil {
		if err := p.onValue(v); err != nil {
			return fmt.Errorf("onValue callback error: %w", err)
		}
	}

	return nil
}

// readChar writes a character to the current buffer.
func (p *StreamParser) readChar(char byte) {
	if len(p.buffers) > 0 {
		p.buffers[len(p.buffers)-1].WriteByte(char)
	} else {
		// Top-level content
		p.topLevelBuf.WriteByte(char)
	}
}

// startScope begins a new scope (object or array).
func (p *StreamParser) startScope(scopeType string) {
	// If starting a scope at top level, move content from topLevelBuf
	if len(p.buffers) == 0 && p.topLevelBuf.Len() > 0 {
		// Should not happen in valid JSON, but handle it
		p.topLevelBuf.Reset()
	}

	p.scopes = append(p.scopes, scopeType)
	p.buffers = append(p.buffers, new(bytes.Buffer))
}

// endScope closes the current scope.
func (p *StreamParser) endScope(scopeType string) error {
	if len(p.scopes) == 0 {
		return fmt.Errorf("unexpected closing '%s' at position %d", p.closingChar(scopeType), p.charCount)
	}

	currentScope := p.scopes[len(p.scopes)-1]
	if currentScope != scopeType {
		return fmt.Errorf("mismatched brackets at position %d: expected '%s', got '%s'",
			p.charCount, p.closingChar(currentScope), p.closingChar(scopeType))
	}

	p.scopes = p.scopes[:len(p.scopes)-1]

	currentBuffer := p.buffers[len(p.buffers)-1]
	p.buffers = p.buffers[:len(p.buffers)-1]

	data := currentBuffer.Bytes()

	// If this is a top-level element, dispatch it
	if len(p.buffers) == 0 {
		var err error
		switch scopeType {
		case "object":
			err = p.dispatchObject(data)
		case "array":
			err = p.dispatchArray(data)
		}
		currentBuffer.Reset()
		return err
	}

	// Otherwise, append to parent buffer
	if len(p.buffers) > 0 {
		p.buffers[len(p.buffers)-1].Write(data)
	}
	currentBuffer.Reset()

	return nil
}

// dispatchObject parses and dispatches an object.
func (p *StreamParser) dispatchObject(data []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		parseErr := fmt.Errorf("unmarshal object at position %d: %w", p.charCount, err)
		p.handleError(parseErr)
		return parseErr
	}

	if p.onObject != nil {
		if err := p.onObject(obj); err != nil {
			return fmt.Errorf("onObject callback error: %w", err)
		}
	}
	return nil
}

// dispatchArray parses and dispatches an array.
func (p *StreamParser) dispatchArray(data []byte) error {
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		parseErr := fmt.Errorf("unmarshal array at position %d: %w", p.charCount, err)
		p.handleError(parseErr)
		return parseErr
	}

	if p.onArray != nil {
		if err := p.onArray(arr); err != nil {
			return fmt.Errorf("onArray callback error: %w", err)
		}
	}
	return nil
}

// handleError calls the error handler if set.
func (p *StreamParser) handleError(err error) {
	if p.onError != nil {
		p.onError(err)
	}
}

// closingChar returns the closing character for a given scope type.
func (p *StreamParser) closingChar(scopeType string) string {
	switch scopeType {
	case "object":
		return "}"
	case "array":
		return "]"
	default:
		return ""
	}
}
