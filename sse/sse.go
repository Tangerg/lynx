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
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// ErrMessageNoContent is returned when attempting to encode a message with no fields.
	// According to the SSE specification, a valid message must contain at least one non-empty field.
	ErrMessageNoContent = errors.New("message has no content")

	// ErrMessageInvalidEventName is returned when attempting to encode a message with an invalid event name.
	// Event names must follow DOM event naming rules.
	ErrMessageInvalidEventName = errors.New("message event name is invalid")
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
	byteLF          = []byte("\n")           // Line feed character
	byteLFLF        = []byte("\n\n")         // Two line feeds indicating message boundary
	byteCR          = []byte("\r")           // Carriage return character
	byteEscapedCR   = []byte("\\r")          // Escaped carriage return
	utf8BomSequence = []byte("\xEF\xBB\xBF") // UTF-8 Byte Order Mark
)

// SSE field names, delimiters, and special characters as defined in the W3C specification.
const (
	fieldID                = "id"     // Unique message identifier
	fieldEvent             = "event"  // Event type
	fieldData              = "data"   // Event payload
	fieldRetry             = "retry"  // Reconnection time in milliseconds
	delimiter              = ":"      // Field name-value delimiter
	whitespace             = " "      // Standard space after delimiter
	invalidUTF8Replacement = "\uFFFD" // Unicode replacement character

	// eventNameMessage is the default event type used when no explicit event is specified.
	// According to the SSE specification, when a message doesn't include an event field,
	// clients should dispatch it using the "message" event type.
	eventNameMessage = "message"
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

// isValidSSEEventName checks if the SSE event name meets the specification requirements.
// If the event name is empty, it's considered valid as the default "message" type will be used.
// Otherwise, it must follow DOM event naming rules.
//
// Valid event name rules:
// - Empty string is valid (default "message" type will be used)
// - Must start with a letter
// - Can only contain letters, digits, underscore, hyphen, and period
// - Cannot contain ".." sequence
// - Cannot start or end with a period
// - Cannot contain any whitespace characters
//
// Examples: "update", "user.created", "system-alert" are valid
// While ".update", "user..profile", "alert!" are invalid
func isValidSSEEventName(eventName string) bool {
	if eventName == "" {
		return true
	}
	return isValidDOMEventName(eventName)
}

// isValidDOMEventName validates event names according to DOM specifications:
// - Must not be empty
// - Must not contain '..' or start/end with '.'
// - Must start with a letter
// - Can only contain letters, digits, underscore, hyphen, or period
// - Cannot contain any whitespace
func isValidDOMEventName(eventName string) bool {
	if eventName == "" {
		return false
	}

	if strings.Contains(eventName, "..") ||
		strings.HasPrefix(eventName, ".") ||
		strings.HasSuffix(eventName, ".") {
		return false
	}

	runes := []rune(eventName)

	if !unicode.IsLetter(runes[0]) {
		return false
	}

	for _, r := range runes {
		if unicode.IsSpace(r) {
			return false
		}
		if unicode.IsLetter(r) ||
			unicode.IsDigit(r) ||
			r == '_' ||
			r == '-' ||
			r == '.' {
			continue
		}
		return false
	}

	return true
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
// Returns an error if the message contains no content or has an invalid event name.
// This method is concurrency-safe and can be called by multiple goroutines.
//
// Boundary conditions:
// - If msg is nil, ErrMessageNoContent will be returned
// - Empty string as Event is valid (default "message" type will be used)
// - Newlines in the Data field will be properly handled as multiline data fields
// - Newlines in ID and Event fields will be escaped as \n
// - Generated message will always end with a blank line, even if no fields are provided
// - If Retry value is negative, it will be ignored
func (e *Encoder) Encode(msg *Message) ([]byte, error) {
	if !isValidSSEEventName(msg.Event) {
		return nil, errors.Join(ErrMessageInvalidEventName, fmt.Errorf("event name: %s", msg.Event))
	}
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
	reader         *bufio.Reader  // Input stream containing SSE messages
	scanner        *bufio.Scanner // Line scanner for the input stream
	lastID         string         // Most recently parsed ID (persists between messages)
	eventBuffer    string         // Buffer for the current event type
	dataBuffer     *bytes.Buffer  // Buffer for the current data payload
	retry          int            // Current reconnection time value
}

// NewDecoder creates a new SSE decoder that processes messages from the provided reader.
// It initializes:
// - A bufio.Reader checks for and skips the UTF-8 Byte Order Mark (BOM) sequence
// - A scanner with custom line splitting for SSE protocol
// - Buffers for accumulating event type and data payloads
// - Internal state for tracking message IDs and retry intervals
// The returned decoder is ready to parse SSE messages using the Next() method.
// Note: The decoder does not close the underlying reader when finished.
func NewDecoder(reader io.Reader) *Decoder {
	d := &Decoder{
		reader:     bufio.NewReader(reader),
		scanner:    bufio.NewScanner(reader),
		dataBuffer: bytes.NewBuffer(make([]byte, 0, 128)),
	}
	d.skipLeadingUTF8BOM()
	d.scanner.Split(d.scanLinesSplit)

	return d
}

// skipLeadingUTF8BOM checks for and skips the UTF-8 Byte Order Mark (BOM) sequence
// at the beginning of the stream if present. According to the SSE specification,
// one leading U+FEFF BOM character must be ignored if present at the start of the stream.
// This method is called once during decoder initialization and does not affect
// subsequent data processing.
func (d *Decoder) skipLeadingUTF8BOM() {
	peek, err := d.reader.Peek(3)
	if err != nil {
		return
	}
	if bytes.Equal(peek, utf8BomSequence) {
		_, _ = d.reader.Discard(3)
	}
}

// scanLinesSplit implements a custom split function for bufio.Scanner to handle various
// line ending patterns in SSE streams. It properly processes:
// - CRLF (\r\n) sequences as a single line break
// - CR (\r) alone as a line break
// - LF (\n) alone as a line break
// - EOF at the end of data
// This ensures compatibility with different server implementations and platforms.
// The function returns the number of bytes to advance, the line token without
// line break characters, and any error encountered.
func (d *Decoder) scanLinesSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.IndexByte(data, byteCR[0]); i >= 0 {
		if i+1 < len(data) && data[i+1] == byteLF[0] {
			return i + 2, data[:i], nil
		}
		return i + 1, data[:i], nil
	}

	if i := bytes.IndexByte(data, byteLF[0]); i >= 0 {
		return i + 1, data[:i], nil
	}

	if atEOF {
		if len(data) > 0 && data[len(data)-1] == byteCR[0] {
			return len(data), data[:len(data)-1], nil
		}
		return len(data), data, nil
	}

	return 0, nil, nil
}

// normalizeValue processes a field value according to the SSE specification:
// - Removes leading whitespace
// - Handles UTF-8 BOM sequences
// - Replaces invalid UTF-8 sequences with the replacement character
func (d *Decoder) normalizeValue(value string) string {
	value = strings.TrimPrefix(value, whitespace)
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, invalidUTF8Replacement)
	}
	return value
}

// hasValidData checks if the data buffer contains actual content beyond just line terminators.
// Since empty data fields still result in a newline character being added to the buffer,
// this method verifies that the buffer contains more than just that single newline.
// Returns true if there is meaningful data to be processed, false otherwise.
func (d *Decoder) hasValidData() bool {
	return d.dataBuffer.Len() > 1 // Buffer length > 1 indicates presence of actual data content
}

// dispatch constructs a message from accumulated field values if valid data is present.
// Returns true if a message was successfully constructed, false otherwise.
// Also resets buffers after attempting to construct a message.
func (d *Decoder) dispatch() bool {
	defer d.resetBuffers()

	if !d.hasValidData() {
		return false
	}
	if !isValidSSEEventName(d.eventBuffer) {
		return false
	}

	d.constructMessage()
	return true
}

// constructMessage builds a Message from the accumulated field values and updates currentMessage.
// Called when a complete SSE message has been parsed (indicated by an empty line).
// This method:
// - Extracts the event type from eventBuffer
// - Retrieves data from dataBuffer, removing any trailing newline
// - Sets the message ID from the lastID (which persists across messages)
// - Sets the retry value for reconnection timing
func (d *Decoder) constructMessage() {
	data := bytes.TrimSuffix(d.dataBuffer.Bytes(), byteLF)
	d.currentMessage.ID = d.lastID
	d.currentMessage.Event = d.eventBuffer
	d.currentMessage.Data = data
	d.currentMessage.Retry = d.retry
}

// resetBuffers clears the temporary buffers and values after a message has been dispatched.
// According to the SSE specification, the lastID field persists between messages until
// explicitly changed by a new ID field, so it is not reset here.
func (d *Decoder) resetBuffers() {
	d.eventBuffer = ""
	d.dataBuffer.Reset()
	d.retry = 0
}

// processLine handles a single line of input from the SSE stream according to the specification:
// - Ignores comment lines (starting with ':')
// - Parses field name and value pairs
// - Updates the appropriate internal state based on the field name:
//   - "id": Updates the lastID field
//   - "event": Appends to the event buffer
//   - "data": Appends to the data buffer with a trailing newline
//   - "retry": Converts to an integer for reconnection timing
func (d *Decoder) processLine(line string) {
	if strings.HasPrefix(line, delimiter) {
		return // Ignore comment lines
	}

	key, value, found := strings.Cut(line, delimiter)
	if !found {
		key = line
		value = ""
	} else {
		value = d.normalizeValue(value)
	}

	switch key {
	case fieldID:
		d.lastID = value // Update the last seen ID
	case fieldEvent:
		if value == "" {
			value = eventNameMessage
		}
		d.eventBuffer = value
	case fieldData:
		d.dataBuffer.WriteString(value)
		// Add newline after each data line, the last \n will be trimmed when constructMessage
		d.dataBuffer.Write(byteLF)
	case fieldRetry:
		retry, err := strconv.Atoi(value) // Parse reconnection time
		if err == nil &&
			retry > 0 {
			d.retry = retry // Update only if positive and valid
		}
	}
}

// Current returns the most recently decoded message.
// Should be called after Next() returns true to access the parsed message.
func (d *Decoder) Current() Message {
	return d.currentMessage
}

// Next advances to the next message in the stream, parsing lines until a complete
// message is found or the stream ends. Returns true if a message was successfully
// decoded, false if the stream ended or an error occurred.
//
// Boundary conditions:
// - Returns false if the stream has ended
// - Returns false and sets internal error state (retrievable via Error()) if an error is encountered
// - Completes parsing of the current message and returns true when an empty line is encountered
// - If an empty line is encountered but there is no current content, continues parsing until a message with content is found
// - If there is an incomplete message at the end of the stream (not terminated by an empty line), still parses and returns that message
// - If a message contains an invalid event name, that message will be ignored and parsing continues
// - If invalid UTF-8 sequences are found, they will be replaced with the U+FFFD character
func (d *Decoder) Next() bool {
	if d.lastError != nil {
		return false
	}

	for d.scanner.Scan() {
		d.lastError = d.scanner.Err()
		if d.lastError != nil {
			return false
		}

		line := d.scanner.Text()

		// Empty line indicates end of message
		if len(line) == 0 {
			if d.dispatch() {
				return true
			}
		}

		d.processLine(line)
	}

	// Handle any final message at end of stream
	if d.dispatch() {
		return true
	}

	// Check for scanner errors
	d.lastError = d.scanner.Err()
	return false
}

// Error returns any error encountered during the decoding process.
// Should be checked after Next() returns false to determine if the stream
// ended normally or due to an error condition.
func (d *Decoder) Error() error {
	return d.lastError
}
