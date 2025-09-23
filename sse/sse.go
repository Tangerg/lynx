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

	// Check for invalid dot patterns
	if strings.Contains(eventName, "..") ||
		strings.HasPrefix(eventName, ".") ||
		strings.HasSuffix(eventName, ".") {
		return false
	}

	eventRunes := []rune(eventName)

	// First character must be a letter
	if !unicode.IsLetter(eventRunes[0]) {
		return false
	}

	// Check all characters for validity
	for _, currentRune := range eventRunes {
		if unicode.IsSpace(currentRune) {
			return false
		}
		if unicode.IsLetter(currentRune) ||
			unicode.IsDigit(currentRune) ||
			currentRune == '_' ||
			currentRune == '-' ||
			currentRune == '.' {
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
func (e *Encoder) isValidMessage(messageToValidate *Message) bool {
	if len(messageToValidate.ID) == 0 &&
		len(messageToValidate.Event) == 0 &&
		len(messageToValidate.Data) == 0 {
		return false
	}
	return true
}

// writeID formats and writes the ID field to the buffer if it contains content,
// escaping any CR and LF characters as required by the specification.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeID(messageID string, outputBuffer *bytes.Buffer) {
	if len(messageID) == 0 {
		return
	}

	outputBuffer.Write(fieldPrefixID)
	outputBuffer.WriteString(lineBreakReplacer.Replace(messageID))
	outputBuffer.Write(byteLF)
}

// writeEvent formats and writes the event field to the buffer if specified,
// escaping any CR and LF characters. When not specified, clients default to "message".
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeEvent(eventName string, outputBuffer *bytes.Buffer) {
	if len(eventName) == 0 {
		return
	}

	outputBuffer.Write(fieldPrefixEvent)
	outputBuffer.WriteString(lineBreakReplacer.Replace(eventName))
	outputBuffer.Write(byteLF)
}

// writeData formats and writes the data field to the buffer,
// handling multiline data by prefixing each line with "data: " and properly escaping CR characters.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeData(messageData []byte, outputBuffer *bytes.Buffer) {
	if len(messageData) == 0 {
		return
	}

	// Escape carriage return characters
	processedData := bytes.ReplaceAll(messageData, byteCR, byteEscapedCR)

	// Split into lines and write each with data prefix
	dataLines := bytes.Split(processedData, byteLF)
	for _, dataLine := range dataLines {
		outputBuffer.Write(fieldPrefixData)
		outputBuffer.Write(dataLine)
		outputBuffer.Write(byteLF)
	}
}

// writeRetry writes the retry field to the buffer if the value is non-zero,
// indicating the time in milliseconds clients should wait before reconnecting.
// This method is concurrency-safe as it operates only on the provided buffer.
func (e *Encoder) writeRetry(retryValue int, outputBuffer *bytes.Buffer) {
	if retryValue == 0 {
		return
	}

	outputBuffer.Write(fieldPrefixRetry)
	outputBuffer.WriteString(strconv.Itoa(retryValue))
	outputBuffer.Write(byteLF)
}

// encodeToBytes formats the message into the SSE wire format according to the specification,
// ensuring each field is properly formatted and terminating the message with a blank line.
// This method is concurrency-safe as it creates a new buffer for each call.
func (e *Encoder) encodeToBytes(messageToEncode *Message) []byte {
	estimatedCapacity := len(messageToEncode.ID) + len(messageToEncode.Event) + 2*len(messageToEncode.Data) + 8
	outputBuffer := bytes.NewBuffer(make([]byte, 0, estimatedCapacity))

	// Write all message fields
	e.writeID(messageToEncode.ID, outputBuffer)
	e.writeEvent(messageToEncode.Event, outputBuffer)
	e.writeData(messageToEncode.Data, outputBuffer)
	e.writeRetry(messageToEncode.Retry, outputBuffer)

	// Terminate message with blank line
	outputBuffer.Write(byteLF)

	return outputBuffer.Bytes()
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
func (e *Encoder) Encode(messageToEncode *Message) ([]byte, error) {
	// Validate event name
	if !isValidSSEEventName(messageToEncode.Event) {
		return nil, errors.Join(ErrMessageInvalidEventName, fmt.Errorf("event name: %s", messageToEncode.Event))
	}

	// Validate message content
	if !e.isValidMessage(messageToEncode) {
		return nil, ErrMessageNoContent
	}

	return e.encodeToBytes(messageToEncode), nil
}

// Decoder processes an SSE stream from an io.Reader, parsing fields and
// detecting message boundaries according to the specification.
type Decoder struct {
	lastError      error          // Most recent error encountered
	currentMessage Message        // Currently decoded message
	streamReader   *bufio.Reader  // Input stream containing SSE messages
	lineScanner    *bufio.Scanner // Line scanner for the input stream
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
func NewDecoder(inputReader io.Reader) *Decoder {
	decoder := &Decoder{
		streamReader: bufio.NewReader(inputReader),
		lineScanner:  bufio.NewScanner(inputReader),
		dataBuffer:   bytes.NewBuffer(make([]byte, 0, 128)),
	}

	// Initialize decoder state
	decoder.skipLeadingUTF8BOM()
	decoder.lineScanner.Split(decoder.scanLinesSplit)

	return decoder
}

// skipLeadingUTF8BOM checks for and skips the UTF-8 Byte Order Mark (BOM) sequence
// at the beginning of the stream if present. According to the SSE specification,
// one leading U+FEFF BOM character must be ignored if present at the start of the stream.
// This method is called once during decoder initialization and does not affect
// subsequent data processing.
func (d *Decoder) skipLeadingUTF8BOM() {
	peekedBytes, err := d.streamReader.Peek(3)
	if err != nil {
		return
	}

	if bytes.Equal(peekedBytes, utf8BomSequence) {
		_, _ = d.streamReader.Discard(3)
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

	// Look for CR character
	if crIndex := bytes.IndexByte(data, byteCR[0]); crIndex >= 0 {
		// Check for CRLF sequence
		if crIndex+1 < len(data) && data[crIndex+1] == byteLF[0] {
			return crIndex + 2, data[:crIndex], nil
		}
		// Just CR
		return crIndex + 1, data[:crIndex], nil
	}

	// Look for LF character
	if lfIndex := bytes.IndexByte(data, byteLF[0]); lfIndex >= 0 {
		return lfIndex + 1, data[:lfIndex], nil
	}

	// Handle EOF
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
func (d *Decoder) normalizeValue(fieldValue string) string {
	normalizedValue := strings.TrimPrefix(fieldValue, whitespace)
	if !utf8.ValidString(normalizedValue) {
		normalizedValue = strings.ToValidUTF8(normalizedValue, invalidUTF8Replacement)
	}
	return normalizedValue
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

	// Check if we have valid data to dispatch
	if !d.hasValidData() {
		return false
	}

	// Validate event name
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
	// Remove trailing newline from data buffer
	messageData := bytes.TrimSuffix(d.dataBuffer.Bytes(), byteLF)

	// Build the message
	d.currentMessage.ID = d.lastID
	d.currentMessage.Event = d.eventBuffer
	d.currentMessage.Data = messageData
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
func (d *Decoder) processLine(inputLine string) {
	// Ignore comment lines
	if strings.HasPrefix(inputLine, delimiter) {
		return
	}

	// Parse field name and value
	fieldName, fieldValue, hasValue := strings.Cut(inputLine, delimiter)
	if !hasValue {
		fieldName = inputLine
		fieldValue = ""
	} else {
		fieldValue = d.normalizeValue(fieldValue)
	}

	// Process based on field name
	switch fieldName {
	case fieldID:
		d.lastID = fieldValue // Update the last seen ID

	case fieldEvent:
		if fieldValue == "" {
			fieldValue = eventNameMessage
		}
		d.eventBuffer = fieldValue

	case fieldData:
		d.dataBuffer.WriteString(fieldValue)
		// Add newline after each data line, the last \n will be trimmed when constructMessage
		d.dataBuffer.Write(byteLF)

	case fieldRetry:
		retryValue, parseErr := strconv.Atoi(fieldValue) // Parse reconnection time
		if parseErr == nil && retryValue > 0 {
			d.retry = retryValue // Update only if positive and valid
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

	// Process lines from the scanner
	for d.lineScanner.Scan() {
		d.lastError = d.lineScanner.Err()
		if d.lastError != nil {
			return false
		}

		currentLine := d.lineScanner.Text()

		// Empty line indicates end of message
		if len(currentLine) == 0 {
			if d.dispatch() {
				return true
			}
			continue
		}

		d.processLine(currentLine)
	}

	// Check for scanner errors
	d.lastError = d.lineScanner.Err()
	if d.lastError != nil {
		return false
	}

	// Handle any final message at end of stream
	if d.dispatch() {
		return true
	}

	return false
}

// Error returns any error encountered during the decoding process.
// Should be checked after Next() returns false to determine if the stream
// ended normally or due to an error condition.
func (d *Decoder) Error() error {
	return d.lastError
}
