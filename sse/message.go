package sse

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

type Message struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	ID    string          `json:"id,omitempty"`
	Retry int             `json:"retry,omitempty"`
}

func (m *Message) Marshal() ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	if m.ID != "" {
		buf.WriteString("id: ")
		buf.WriteString(m.ID)
		buf.WriteString("\n")
	}

	if m.Event != "" {
		buf.WriteString("event: ")
		buf.WriteString(m.Event)
		buf.WriteString("\n")
	}

	if len(m.Data) > 0 {
		buf.WriteString("data: ")
		buf.Write(m.Data)
		buf.WriteString("\n")
	}

	if m.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(m.Retry))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")

	return buf.Bytes(), nil
}

type messageDecoder struct {
	currentMessage Message
	readCloser     io.ReadCloser
	scanner        *bufio.Scanner
	error          error
}

func newMessageDecoder(readCloser io.ReadCloser) *messageDecoder {
	return &messageDecoder{
		readCloser: readCloser,
		scanner:    bufio.NewScanner(readCloser),
	}
}

func (e *messageDecoder) Current() Message {
	return e.currentMessage
}

func (e *messageDecoder) Next() bool {
	if e.error != nil {
		return false
	}

	var (
		event            = ""
		data             = bytes.NewBuffer(nil)
		id               = ""
		retry            = 0
		consecutiveEmpty = 0
	)

	for e.scanner.Scan() {
		content := e.scanner.Text()

		if len(content) == 0 {
			consecutiveEmpty++
			if consecutiveEmpty >= 2 {
				e.error = e.Close()
				return false
			}

			if event != "" || data.Len() > 0 || id != "" || retry != 0 {
				e.currentMessage = Message{
					Event: event,
					Data:  data.Bytes(),
					ID:    id,
					Retry: retry,
				}
				consecutiveEmpty = 0
				return true
			}
			continue
		}

		key, value, found := strings.Cut(content, ":")
		if !found {
			continue
		}

		value = strings.TrimPrefix(value, " ")

		switch key {
		case "event":
			event = value
		case "id":
			id = value
		case "retry":
			retry, _ = strconv.Atoi(value)
		case "data":
			_, e.error = data.WriteString(value)
			if e.error != nil {
				break
			}
			_, e.error = data.WriteRune('\n')
			if e.error != nil {
				break
			}
		}

	}

	return false
}

func (e *messageDecoder) Close() error {
	return e.readCloser.Close()
}

func (e *messageDecoder) Error() error {
	return e.error
}
