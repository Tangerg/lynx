// Package sse implements the Server-Sent Events (SSE) protocol according to the W3C specification.
// See: https://www.w3.org/TR/2009/WD-eventsource-20091029/
//
// SSE is a one-way communication protocol that allows servers to push real-time updates
// to clients over a single HTTP connection. This package provides:
//
// - Message encoding into the SSE wire format
// - Message decoding from HTTP response streams
// - Support for all SSE fields: id, event, data, and retry
// - Multiline data processing according to specification
// - Message validation and sanitization
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

// lineBreakReplacer handles escaping of CR and LF characters in fields like id and event,
// as required by the SSE specification.
var (
	lineBreakReplacer = strings.NewReplacer(
		"\n", "\\n",
		"\r", "\\r",
	)
)

// Byte constants for message processing to improve performance.
var (
	byteLF        = []byte("\n")   // Line feed character
	byteLFLF      = []byte("\n\n") // Two line feeds indicating message boundary
	byteCR        = []byte("\r")   // Carriage return character
	byteEscapedCR = []byte("\\r")  // Escaped carriage return
)

// SSE field names, delimiters, and special characters as defined in the W3C specification.
const (
	fieldID                = "id"           // Unique message identifier
	fieldEvent             = "event"        // Event type
	fieldData              = "data"         // Event payload
	fieldRetry             = "retry"        // Reconnection time in milliseconds
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
	Retry int    // Reconnection time in milliseconds
}

// Encoder handles the conversion of Message objects to the SSE wire format.
// It is concurrency-safe for use by multiple goroutines simultaneously.
type Encoder struct{}

// NewEncoder creates a new SSE message encoder.
// The returned encoder is safe for concurrent use across multiple goroutines.
func NewEncoder() *Encoder {
	return &Encoder{}
}

// isValidMessage verifies that at least one field in the message contains content.
// According to the SSE specification, a message must have at least one non-empty field.
// This method is concurrency-safe as it doesn't modify encoder state.
func (e *Encoder) isValidMessage(msg *Message) bool {
	if len(msg.ID) == 0 &&
		len(msg.Event) == 0 &&
		len(msg.Data) == 0 {
		return false
	}
	return true
}

// writeID formats and writes the ID field to the buffer if it contains content,
// escaping any CR and LF characters as required by the specification.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeID(id string, buffer *bytes.Buffer) {
	if len(id) == 0 {
		return
	}

	buffer.Write(fieldPrefixID)
	buffer.WriteString(lineBreakReplacer.Replace(id))
	buffer.Write(byteLF)
}

// writeEvent formats and writes the event field to the buffer if specified,
// escaping any CR and LF characters. When not specified, clients default to "message".
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeEvent(event string, buffer *bytes.Buffer) {
	if len(event) == 0 {
		return
	}

	buffer.Write(fieldPrefixEvent)
	buffer.WriteString(lineBreakReplacer.Replace(event))
	buffer.Write(byteLF)
}

// writeData formats and writes the data field to the buffer,
// handling multiline data by prefixing each line with "data: " and properly escaping CR characters.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeData(data []byte, buffer *bytes.Buffer) {
	if len(data) == 0 {
		return
	}

	processedData := bytes.ReplaceAll(data, byteCR, byteEscapedCR)

	lines := bytes.Split(processedData, byteLF)
	for _, line := range lines {
		buffer.Write(fieldPrefixData)
		buffer.Write(line)
		buffer.Write(byteLF)
	}
}

// writeRetry writes the retry field to the buffer if the value is non-zero,
// indicating the time in milliseconds clients should wait before reconnecting.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeRetry(retry int, buffer *bytes.Buffer) {
	if retry == 0 {
		return
	}

	buffer.Write(fieldPrefixRetry)
	buffer.WriteString(strconv.Itoa(retry))
	buffer.Write(byteLF)
}

// encodeToBytes formats the message into the SSE wire format according to the specification,
// ensuring each field is properly formatted and terminating the message with a blank line.
// This method is concurrency-safe as it creates a new buffer for each call.
func (e *Encoder) encodeToBytes(msg *Message) []byte {
	buffer := bytes.NewBuffer(make([]byte, 0, len(msg.ID)+len(msg.Event)+2*len(msg.Data)+8))

	e.writeID(msg.ID, buffer)
	e.writeEvent(msg.Event, buffer)
	e.writeData(msg.Data, buffer)
	e.writeRetry(msg.Retry, buffer)
	buffer.Write(byteLF) // Terminate message with blank line

	return buffer.Bytes()
}

// Encode validates and encodes a message into the SSE wire format.
// Returns an error if the message contains no content.
// This method is concurrency-safe and can be called by multiple goroutines.
func (e *Encoder) Encode(msg *Message) ([]byte, error) {
	if !e.isValidMessage(msg) {
		return nil, ErrMessageNoContent
	}

	return e.encodeToBytes(msg), nil
}

// Decoder processes an SSE stream from an io.Reader, parsing fields and
// detecting message boundaries according to the specification.
type Decoder struct {
	lastError      error          // Most recent error encountered
	currentMessage Message        // Currently decoded message
	reader         io.Reader      // Input stream containing SSE messages
	scanner        *bufio.Scanner // Line scanner for the input stream
	lastID         string         // Most recently parsed ID (persists between messages)
	eventBuffer    *bytes.Buffer  // Buffer for the current event type
	dataBuffer     *bytes.Buffer  // Buffer for the current data payload
	retry          int            // Current reconnection time value
}

// NewDecoder creates a new SSE decoder for processing messages from the specified reader.
func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{
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
func (e *Decoder) normalizeValue(value string) string {
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
func (e *Decoder) hasContent() bool {
	if e.eventBuffer.Len() == 0 &&
		e.dataBuffer.Len() == 0 {
		return false
	}
	return true
}

// constructMessage builds a Message from the accumulated field values and updates currentMessage.
// Called when a complete SSE message has been parsed (indicated by an empty line).
// This method:
// - Extracts the event type from eventBuffer
// - Retrieves data from dataBuffer, removing any trailing newline
// - Sets the message ID from the lastID (which persists across messages)
// - Sets the retry value for reconnection timing
// - Resets buffers after constructing the message
func (e *Decoder) constructMessage() {
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
func (e *Decoder) resetBuffers() {
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
func (e *Decoder) processLine(line string) {
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
func (e *Decoder) Current() Message {
	return e.currentMessage
}

// Next advances to the next message in the stream, parsing lines until a complete
// message is found or the stream ends. Returns true if a message was successfully
// decoded, false if the stream ended or an error occurred.
func (e *Decoder) Next() bool {
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
func (e *Decoder) Error() error {
	return e.lastError
}
