package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode"
)

// StreamParserConfig configures a [StreamParser].
type StreamParserConfig struct {
	// BufferSize is the read-buffer size in bytes. Defaults to 4096.
	BufferSize int
	// Reader is the source of JSON bytes. Required.
	Reader io.Reader
	// OnArray is called once per top-level JSON array.
	OnArray func([]any) error
	// OnObject is called once per top-level JSON object.
	OnObject func(map[string]any) error
	// OnValue is called once per top-level primitive value.
	OnValue func(any) error
	// OnError receives parse errors before they are returned.
	OnError func(error)
}

const defaultParserBufSize = 4096

// validate fills in defaults and reports configuration errors.
func (c StreamParserConfig) Validate() error {
	if c.Reader == nil {
		return errors.New("json: reader required")
	}
	if c.BufferSize <= 0 {
		c.BufferSize = defaultParserBufSize
	}
	return nil
}

// StreamParser parses a JSON stream incrementally and emits each
// top-level value through the configured callbacks. It is not safe
// for concurrent use; create one parser per stream.
//
// Example:
//
//	p, _ := json.NewStreamParser(json.StreamParserConfig{
//	    Reader: r,
//	    OnObject: func(o map[string]any) error { handle(o); return nil },
//	})
//	if err := p.Parse(); err != nil {
//	    return err
//	}
type StreamParser struct {
	bufferSize int
	scopes     []string
	reader     io.Reader
	buffers    []*bytes.Buffer
	topBuf     *bytes.Buffer
	inString   bool
	escaped    bool
	pos        int
	onArray    func([]any) error
	onObject   func(map[string]any) error
	onValue    func(any) error
	onError    func(error)
}

// NewStreamParser returns a parser configured by config.
func NewStreamParser(config StreamParserConfig) (*StreamParser, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &StreamParser{
		bufferSize: config.BufferSize,
		scopes:     make([]string, 0, 8),
		reader:     config.Reader,
		buffers:    make([]*bytes.Buffer, 0, 8),
		topBuf:     new(bytes.Buffer),
		onArray:    config.OnArray,
		onObject:   config.OnObject,
		onValue:    config.OnValue,
		onError:    config.OnError,
	}, nil
}

// Parse reads from the configured reader until io.EOF, dispatching
// each top-level value to the appropriate callback. It returns an
// error on read failure, callback failure, or malformed JSON.
func (p *StreamParser) Parse() error {
	buf := make([]byte, p.bufferSize)
	for {
		n, err := p.reader.Read(buf)
		if n > 0 {
			if err := p.processBytes(buf[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if err := p.flushTopLevel(); err != nil {
					return err
				}
				if len(p.scopes) > 0 {
					return fmt.Errorf("json: unexpected EOF: unclosed %s", p.scopes[len(p.scopes)-1])
				}
				return nil
			}
			return fmt.Errorf("json: read: %w", err)
		}
	}
}

// processBytes feeds each byte through [StreamParser.processChar].
func (p *StreamParser) processBytes(data []byte) error {
	for _, b := range data {
		if err := p.processChar(b); err != nil {
			return err
		}
	}
	return nil
}

// processChar advances the parser state for a single byte.
func (p *StreamParser) processChar(c byte) error {
	p.pos++
	if c == '"' && !p.escaped {
		p.inString = !p.inString
		p.write(c)
		return nil
	}
	if p.inString {
		p.write(c)
		switch {
		case p.escaped:
			p.escaped = false
		case c == '\\':
			p.escaped = true
		}
		return nil
	}
	switch c {
	case '{':
		p.startScope("object")
		p.write(c)
	case '}':
		p.write(c)
		return p.endScope("object")
	case '[':
		p.startScope("array")
		p.write(c)
	case ']':
		p.write(c)
		return p.endScope("array")
	case ',':
		if len(p.scopes) == 0 {
			return p.flushTopLevel()
		}
		p.write(c)
	default:
		if unicode.IsSpace(rune(c)) {
			if len(p.scopes) == 0 && p.topBuf.Len() > 0 && p.isCompletePrimitive() {
				return p.flushTopLevel()
			}
			if len(p.scopes) > 0 {
				p.write(c)
			}
		} else {
			p.write(c)
		}
	}
	return nil
}

// isCompletePrimitive checks whether the top-level buffer parses as
// a JSON value.
func (p *StreamParser) isCompletePrimitive() bool {
	data := bytes.TrimSpace(p.topBuf.Bytes())
	if len(data) == 0 {
		return false
	}
	var v any
	return json.Unmarshal(data, &v) == nil
}

// flushTopLevel parses and dispatches a primitive top-level value.
func (p *StreamParser) flushTopLevel() error {
	data := bytes.TrimSpace(p.topBuf.Bytes())
	if len(data) == 0 {
		return nil
	}
	defer p.topBuf.Reset()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("json: unmarshal top-level @%d: %w", p.pos, err)
	}
	if p.onValue != nil {
		if err := p.onValue(v); err != nil {
			return fmt.Errorf("json: onValue: %w", err)
		}
	}
	return nil
}

// write appends c to the current scope's buffer (or top-level if
// outside any scope).
func (p *StreamParser) write(c byte) {
	if n := len(p.buffers); n > 0 {
		p.buffers[n-1].WriteByte(c)
		return
	}
	p.topBuf.WriteByte(c)
}

// startScope pushes a new "object" or "array" scope.
func (p *StreamParser) startScope(kind string) {
	if len(p.buffers) == 0 && p.topBuf.Len() > 0 {
		p.topBuf.Reset()
	}
	p.scopes = append(p.scopes, kind)
	p.buffers = append(p.buffers, new(bytes.Buffer))
}

// endScope pops a scope, validates the matching delimiter, and
// dispatches the completed value when the stack returns to top level.
func (p *StreamParser) endScope(kind string) error {
	if len(p.scopes) == 0 {
		return fmt.Errorf("json: unexpected '%s' @%d", closingChar(kind), p.pos)
	}
	cur := p.scopes[len(p.scopes)-1]
	if cur != kind {
		return fmt.Errorf("json: mismatched brackets @%d: want '%s', got '%s'", p.pos, closingChar(cur), closingChar(kind))
	}
	p.scopes = p.scopes[:len(p.scopes)-1]

	curBuf := p.buffers[len(p.buffers)-1]
	p.buffers = p.buffers[:len(p.buffers)-1]
	data := curBuf.Bytes()

	if len(p.buffers) == 0 {
		var err error
		switch kind {
		case "object":
			err = p.dispatchObject(data)
		case "array":
			err = p.dispatchArray(data)
		}
		curBuf.Reset()
		return err
	}
	p.buffers[len(p.buffers)-1].Write(data)
	curBuf.Reset()
	return nil
}

// dispatchObject decodes data as an object and invokes onObject.
func (p *StreamParser) dispatchObject(data []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		parseErr := fmt.Errorf("json: unmarshal object @%d: %w", p.pos, err)
		p.notify(parseErr)
		return parseErr
	}
	if p.onObject != nil {
		if err := p.onObject(obj); err != nil {
			return fmt.Errorf("json: onObject: %w", err)
		}
	}
	return nil
}

// dispatchArray decodes data as an array and invokes onArray.
func (p *StreamParser) dispatchArray(data []byte) error {
	var arr []any
	if err := json.Unmarshal(data, &arr); err != nil {
		parseErr := fmt.Errorf("json: unmarshal array @%d: %w", p.pos, err)
		p.notify(parseErr)
		return parseErr
	}
	if p.onArray != nil {
		if err := p.onArray(arr); err != nil {
			return fmt.Errorf("json: onArray: %w", err)
		}
	}
	return nil
}

// notify forwards err to OnError if configured.
func (p *StreamParser) notify(err error) {
	if p.onError != nil {
		p.onError(err)
	}
}

// closingChar returns the closing token for a scope kind.
func closingChar(kind string) string {
	switch kind {
	case "object":
		return "}"
	case "array":
		return "]"
	}
	return ""
}
