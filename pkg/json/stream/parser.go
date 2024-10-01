package stream

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

type JSONParser struct {
	readbufferSize int
	scopes         []string
	buffers        []*bytes.Buffer
	OnObject       func(map[string]any)
	OnArray        func([]any)
}

func NewJSONParser(size int) *JSONParser {
	return &JSONParser{
		readbufferSize: size,
	}
}

func (p *JSONParser) Parse(input io.Reader) error {
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

func (p *JSONParser) readChar(char byte) {
	if len(p.scopes) == 0 {
		return
	}

	currentBuffer := p.buffers[len(p.buffers)-1]
	currentBuffer.WriteByte(char)
}

func (p *JSONParser) startScope(scopeType string) {
	p.scopes = append(p.scopes, scopeType)
	p.buffers = append(p.buffers, new(bytes.Buffer))
}

func (p *JSONParser) endScope(scopeType string) {
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

func (p *JSONParser) processObjectBuffer(buffer *bytes.Buffer) {
	var obj map[string]interface{}
	err := json.Unmarshal(buffer.Bytes(), &obj)
	if err != nil {
		return
	}
	if p.OnObject != nil {
		p.OnObject(obj)
	}
}

func (p *JSONParser) processArrayBuffer(buffer *bytes.Buffer) {
	var arr []any
	err := json.Unmarshal(buffer.Bytes(), &arr)
	if err != nil {
		return
	}
	if p.OnArray != nil {
		p.OnArray(arr)
	}
}
