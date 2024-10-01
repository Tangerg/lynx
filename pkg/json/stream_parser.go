package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode"
)

const (
	array  = "array"
	object = "object"
)

type StreamParser struct {
	readbufferSize int
	scopes         []string
	buffers        []*bytes.Buffer
	OnObject       func(map[string]any)
	OnArray        func([]any)
}

func NewStreamParser(size int) *StreamParser {
	return &StreamParser{
		readbufferSize: size,
	}
}

func (p *StreamParser) Parse(input io.Reader) error {
	if input == nil {
		return errors.New("input reader cannot be nil")
	}

	defer func() {
		clear(p.scopes[0:len(p.scopes)])
		clear(p.buffers[0:len(p.buffers)])
	}()

	buf := make([]byte, p.readbufferSize)
	for {
		n, err := input.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		for i := 0; i < n; i++ {
			char := buf[i]
			if unicode.IsSpace(rune(char)) {
				continue
			}

			switch char {
			case '{':
				p.startScope(object)
				p.readChar(char)
			case '}':
				p.readChar(char)
				p.endScope(object)
			case '[':
				p.startScope(array)
				p.readChar(char)
			case ']':
				p.readChar(char)
				p.endScope(array)
			default:
				p.readChar(char)
			}
		}
	}
}

func (p *StreamParser) readChar(char byte) {
	if len(p.scopes) == 0 {
		return
	}

	currentBuffer := p.buffers[len(p.buffers)-1]
	currentBuffer.WriteByte(char)
}

func (p *StreamParser) startScope(scopeType string) {
	p.scopes = append(p.scopes, scopeType)
	p.buffers = append(p.buffers, new(bytes.Buffer))
}

func (p *StreamParser) endScope(scopeType string) {
	if len(p.scopes) == 0 {
		return
	}
	if p.scopes[len(p.scopes)-1] != scopeType {
		return
	}

	p.scopes = p.scopes[:len(p.scopes)-1]

	currentBuffer := p.buffers[len(p.buffers)-1]
	defer currentBuffer.Reset()

	p.buffers = p.buffers[:len(p.buffers)-1]

	if len(p.buffers) == 0 {
		switch scopeType {
		case object:
			p.processObjectBuffer(currentBuffer)
		case array:
			p.processArrayBuffer(currentBuffer)
		}
	} else {
		p.buffers[len(p.buffers)-1].Write(currentBuffer.Bytes())
	}
}

func (p *StreamParser) processObjectBuffer(buffer *bytes.Buffer) {
	var obj map[string]interface{}
	err := json.Unmarshal(buffer.Bytes(), &obj)
	if err != nil {
		return
	}
	if p.OnObject != nil {
		p.OnObject(obj)
	}
}

func (p *StreamParser) processArrayBuffer(buffer *bytes.Buffer) {
	var arr []any
	err := json.Unmarshal(buffer.Bytes(), &arr)
	if err != nil {
		return
	}
	if p.OnArray != nil {
		p.OnArray(arr)
	}
}
