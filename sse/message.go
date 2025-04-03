// Package sse implements the Server-Sent Events (SSE)  protocol according to the W3C specification.
// see w3c doc https://www.w3.org/TR/2009/WD-eventsource-20091029/
// SSE is a one-way communication protocol that allows servers to push real-time updates
// to clients over a single HTTP connection.
//
// This package provides functionality to:
// - Encode SSE messages into the required wire format
// - Decode SSE messages from an HTTP response stream
// - Handle all essential SSE fields: id, event, data, and retry
// - Process multiline data according to the specification
// - Validate and sanitize messages
package sse

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrMessageNoContent is returned when attempting to encode a message with no fields.
// According to the SSE specification, a valid message must contain at least one non-empty field.
var (
	ErrMessageNoContent = errors.New("message has no content")
)

// lineBreakReplacer handles the escaping of CR and LF characters in fields such as id and event,
// as required by the SSE specification.
var (
	lineBreakReplacer = strings.NewReplacer(
		"\n", "\\n",
		"\r", "\\r",
	)
)

// Predefined byte constants for message processing to improve performance.
var (
	byteLF        = []byte("\n")   // Line feed character
	byteLFLF      = []byte("\n\n") // Two line feeds indicating message boundary
	byteCR        = []byte("\r")   // Carriage return character
	byteEscapedCR = []byte("\\r")  // Escaped carriage return
)

// Constants for SSE field names, delimiters, and special characters as defined in the W3C specification.
const (
	fieldID                = "id"           // ID field identifier
	fieldEvent             = "event"        // Event type identifier
	fieldData              = "data"         // Data payload identifier
	fieldRetry             = "retry"        // Reconnection time identifier
	delimiter              = ":"            // Field name-value delimiter
	whitespace             = " "            // Standard space after delimiter
	invalidUTF8Replacement = "\uFFFD"       // Unicode replacement character
	utf8BomSequence        = "\xEF\xBB\xBF" // UTF-8 Byte Order Mark
)

// Precomputed byte arrays for field prefixes to optimize message encoding.
var (
	fieldPrefixID    = []byte(fieldID + delimiter + whitespace)
	fieldPrefixEvent = []byte(fieldEvent + delimiter + whitespace)
	fieldPrefixData  = []byte(fieldData + delimiter + whitespace)
	fieldPrefixRetry = []byte(fieldRetry + delimiter + whitespace)
)

// Message represents a Server-Sent Event with all fields defined in the SSE specification:
// - ID: Uniquely identifies the event and enables connection resumption
// - Event: Defines the event type (defaults to "message" if not specified)
// - Data: Contains the event payload
// - Retry: Specifies the reconnection time in milliseconds
type Message struct {
	ID    string // Message identifier
	Event string // Message type
	Data  []byte // Message payload
	Retry int    // Message Reconnection time in milliseconds
}

// messageEncoder handles the conversion of Message objects to the SSE wire format
// while maintaining an internal buffer to reduce memory allocations.
type messageEncoder struct {
	buffer *bytes.Buffer // Internal buffer for message construction
}

// newMessageEncoder creates a new SSE message encoder with a pre-allocated buffer
// to minimize reallocations during encoding operations.
func newMessageEncoder() *messageEncoder {
	return &messageEncoder{
		buffer: bytes.NewBuffer(make([]byte, 0, 512)),
	}
}

// isValidMessage verifies that at least one field in the message contains content.
// According to the SSE specification, a message must have at least one non-empty field.
func (e *messageEncoder) isValidMessage(msg *Message) bool {
	if len(msg.ID) == 0 &&
		len(msg.Event) == 0 &&
		len(msg.Data) == 0 {
		return false
	}
	return true
}

// writeID formats and writes the ID field to the buffer if it contains content,
// escaping any CR and LF characters as required by the specification.
func (e *messageEncoder) writeID(id string) {
	if len(id) == 0 {
		return
	}

	e.buffer.Write(fieldPrefixID)
	e.buffer.WriteString(lineBreakReplacer.Replace(id))
	e.buffer.Write(byteLF)
}

// writeEvent formats and writes the event field to the buffer if specified,
// escaping any CR and LF characters. When not specified, clients default to "message".
func (e *messageEncoder) writeEvent(event string) {
	if len(event) == 0 {
		return
	}

	e.buffer.Write(fieldPrefixEvent)
	e.buffer.WriteString(lineBreakReplacer.Replace(event))
	e.buffer.Write(byteLF)
}

// writeData formats and writes the data field to the buffer,
// handling multiline data by prefixing each line with "data: " and properly escaping CR characters.
func (e *messageEncoder) writeData(data []byte) {
	if len(data) == 0 {
		return
	}

	processedData := bytes.ReplaceAll(data, byteCR, byteEscapedCR)

	lines := bytes.Split(processedData, byteLF)
	for _, line := range lines {
		e.buffer.Write(fieldPrefixData)
		e.buffer.Write(line)
		e.buffer.Write(byteLF)
	}
}

// writeRetry writes the retry field to the buffer if the value is non-zero,
// indicating the time in milliseconds clients should wait before reconnecting.
func (e *messageEncoder) writeRetry(retry int) {
	if retry == 0 {
		return
	}

	e.buffer.Write(fieldPrefixRetry)
	e.buffer.WriteString(strconv.Itoa(retry))
	e.buffer.Write(byteLF)
}

// encodeToBytes formats the message into the SSE wire format according to the specification,
// ensuring each field is properly formatted and terminating the message with a blank line.
func (e *messageEncoder) encodeToBytes(msg *Message) []byte {
	defer e.buffer.Reset()

	e.writeID(msg.ID)
	e.writeEvent(msg.Event)
	e.writeData(msg.Data)
	e.writeRetry(msg.Retry)
	e.buffer.Write(byteLF) // Terminate message with blank line

	return e.buffer.Bytes()
}

// Encode validates and encodes a message into the SSE wire format.
// Returns an error if the message contains no content.
func (e *messageEncoder) Encode(msg *Message) ([]byte, error) {
	if !e.isValidMessage(msg) {
		return nil, ErrMessageNoContent
	}

	return e.encodeToBytes(msg), nil
}

// messageDecoder processes an SSE stream from an io.Reader, parsing fields and
// detecting message boundaries according to the specification.
type messageDecoder struct {
	lastError      error          // Most recent error encountered
	currentMessage Message        // Currently decoded message
	reader         io.Reader      // Input stream containing SSE messages
	scanner        *bufio.Scanner // Line scanner for the input stream
	lastID         string         // Most recently parsed ID (persists between messages)
	eventBuffer    *bytes.Buffer  // Buffer for the current event type
	dataBuffer     *bytes.Buffer  // Buffer for the current data payload
	retry          int            // Current reconnection time value
}

// newMessageDecoder creates a new SSE decoder for processing messages from the specified reader.
func newMessageDecoder(reader io.Reader) *messageDecoder {
	return &messageDecoder{
		reader:      reader,
		scanner:     bufio.NewScanner(reader),
		eventBuffer: bytes.NewBuffer(make([]byte, 0, 64)),
		dataBuffer:  bytes.NewBuffer(make([]byte, 0, 128)),
	}
}

// normalizeValue processes a field value according to the SSE specification:
// - Removes leading whitespace
// - Handles UTF-8 BOM sequences
// - Replaces invalid UTF-8 sequences with the replacement character
func (e *messageDecoder) normalizeValue(value string) string {
	value = strings.TrimPrefix(value, whitespace)
	value = strings.TrimPrefix(value, utf8BomSequence)
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, invalidUTF8Replacement)
	}
	return value
}

// hasContent determines if the current message contains any data to dispatch.
// According to the SSE specification, a message should be dispatched only if
// it contains at least one non-empty field (either event or data).
func (e *messageDecoder) hasContent() bool {
	if e.eventBuffer.Len() == 0 &&
		e.dataBuffer.Len() == 0 {
		return false
	}
	return true
}

// constructMessage builds a Message from the accumulated field values and updates currentMessage.
// Called when a complete SSE message has been parsed (indicated by an empty line).
// The method:
// - Extracts the event type from eventBuffer
// - Retrieves data from dataBuffer, removing any trailing newline
// - Sets the message ID from the lastID (which persists across messages)
// - Sets the retry value for reconnection timing
// - Resets buffers after constructing the message
func (e *messageDecoder) constructMessage() {
	defer e.resetBuffers()

	event := e.eventBuffer.String()
	data := bytes.TrimSuffix(e.dataBuffer.Bytes(), byteLF)
	e.currentMessage.ID = e.lastID
	e.currentMessage.Event = event
	e.currentMessage.Data = data
	e.currentMessage.Retry = e.retry
}

// resetBuffers clears the temporary buffers and values after a message has been dispatched.
// According to the SSE specification, the lastID field persists between messages until
// explicitly changed by a new ID field, so it is not reset here.
func (e *messageDecoder) resetBuffers() {
	e.eventBuffer.Reset()
	e.dataBuffer.Reset()
	e.retry = 0
}

// processLine handles a single line of input from the SSE stream according to the specification:
// - Ignores comment lines (starting with ':')
// - Parses field name and value pairs
// - Updates the appropriate internal state based on the field name:
//   - "id": Updates the lastID field
//   - "event": Appends to the event buffer
//   - "data": Appends to the data buffer with a trailing newline
//   - "retry": Converts to an integer for reconnection timing
func (e *messageDecoder) processLine(line string) {
	if strings.HasPrefix(line, delimiter) {
		return // Ignore comment lines
	}

	key, value, found := strings.Cut(line, delimiter)
	if !found {
		key = line
		value = ""
	} else {
		value = e.normalizeValue(value)
	}

	switch key {
	case fieldID:
		e.lastID = value // Update the last seen ID
	case fieldEvent:
		e.eventBuffer.WriteString(value) // Store event type
	case fieldData:
		e.dataBuffer.WriteString(value) // Append data content
		e.dataBuffer.Write(byteLF)      // Add newline after each data line
	case fieldRetry:
		retry, _ := strconv.Atoi(value) // Parse reconnection time
		if retry > 0 {
			e.retry = retry // Update only if positive
		}
	}
}

// Current returns the most recently decoded message.
// Should be called after Next() returns true to access the parsed message.
func (e *messageDecoder) Current() Message {
	return e.currentMessage
}

// Next advances to the next message in the stream, parsing lines until a complete
// message is found or the stream ends. Returns true if a message was successfully
// decoded, false if the stream ended or an error occurred.
func (e *messageDecoder) Next() bool {
	if e.lastError != nil {
		return false
	}

	var insideMessageBlock = false // Tracks if we've started processing a message

	for e.scanner.Scan() {
		line := e.scanner.Text()

		// Empty line indicates end of message
		if len(line) == 0 {
			if !insideMessageBlock {
				continue // Skip leading empty lines
			}
			if !e.hasContent() {
				continue
			}
			e.constructMessage()
			return true
		}

		insideMessageBlock = true
		e.processLine(line)
	}

	// Handle any final message at end of stream
	if e.hasContent() {
		e.constructMessage()
		return true
	}

	// Check for scanner errors
	e.lastError = e.scanner.Err()
	return false
}

// Error returns any error encountered during the decoding process.
// Should be checked after Next() returns false to determine if the stream
// ended normally or due to an error condition.
func (e *messageDecoder) Error() error {
	return e.lastError
}
