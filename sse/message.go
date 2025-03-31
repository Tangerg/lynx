// Package sse implements the Server-Sent Events (SSE) protocol according to the W3C specification.
// SSE is a one-way communication protocol allowing servers to push real-time updates
// to clients via a single HTTP connection.
//
// This package provides functionality to:
// - Encode SSE messages into the required wire format
// - Decode SSE messages from an HTTP response stream
// - Handle all essential SSE fields: id, event, data, and retry
// - Process multiline data appropriately
// - Validate and sanitize messages as per the specification
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

// ErrMessageNoContent is returned for attempting to encode a message with no fields.
// According to the SSE specification, a valid message must have at least one non-empty field.
var (
	ErrMessageNoContent = errors.New("message has no content")
)

// lineBreakReplacer manages the escaping of CR and LF characters in fields such as id and event,
// as required by the SSE specification.
var (
	lineBreakReplacer = strings.NewReplacer(
		"\n", "\\n",
		"\r", "\\r",
	)
)

// Predefined byte constants used for constructing and processing messages to optimize performance.
var (
	byteLF        = []byte("\n")   // Line feed character
	byteLFLF      = []byte("\n\n") // Two line feed characters indicating message boundary
	byteCR        = []byte("\r")   // Carriage return character
	byteEscapedCR = []byte("\\r")  // Escaped carriage return
)

// Constants for SSE field names, delimiters, and special characters per the W3C specification.
const (
	fieldID                = "id"           // ID field name
	fieldEvent             = "event"        // Event field name
	fieldData              = "data"         // Data field name
	fieldRetry             = "retry"        // Retry field name
	delimiter              = ":"            // Delimiter between field name and value
	whitespace             = " "            // Space character follows delimiter by convention
	invalidUTF8Replacement = "\uFFFD"       // Replacement character for invalid UTF-8 sequences
	utf8BomSequence        = "\xEF\xBB\xBF" // Byte Order Mark (BOM) for UTF-8
)

// Preassembled byte arrays representing each SSE field prefix for efficient encoding.
var (
	fieldPrefixID    = []byte(fieldID + delimiter + whitespace)
	fieldPrefixEvent = []byte(fieldEvent + delimiter + whitespace)
	fieldPrefixData  = []byte(fieldData + delimiter + whitespace)
	fieldPrefixRetry = []byte(fieldRetry + delimiter + whitespace)
)

// Message represents a Server-Sent Event, aligning closely with the SSE specification.
// Fields include:
// - ID: Enables event tracking and connection resumption
// - Event: Categorizes the event type; defaults to "message" if not specified
// - Data: Payload of the event
// - Retry: Specifies reconnection time in milliseconds
type Message struct {
	ID    string // Event ID
	Event string // Event type
	Data  []byte // Event payload
	Retry int    // Time in milliseconds for reconnection attempts
}

// messageEncoder converts Message objects to the SSE wire format, maintaining an internal buffer
// to minimize memory allocations. Note: This encoder is single-threaded.
type messageEncoder struct {
	buffer *bytes.Buffer // Buffer for message construction
}

// newMessageEncoder initializes a new SSE message encoder with a default buffer size,
// reducing the need for resizing during encoding.
func newMessageEncoder() *messageEncoder {
	return &messageEncoder{
		buffer: bytes.NewBuffer(make([]byte, 0, 512)),
	}
}

// isValidMessage ensures at least one field in the message is non-empty.
// A message without content is invalid as per the SSE specification.
func (e *messageEncoder) isValidMessage(msg *Message) bool {
	if len(msg.ID) == 0 &&
		len(msg.Event) == 0 &&
		len(msg.Data) == 0 {
		return false
	}
	return true
}

// writeID writes the ID field to the buffer if it's non-empty,
// escaping CR and LF characters as required by the specification.
func (e *messageEncoder) writeID(id string) {
	if len(id) == 0 {
		return
	}

	e.buffer.Write(fieldPrefixID)
	e.buffer.WriteString(lineBreakReplacer.Replace(id))
	e.buffer.Write(byteLF)
}

// writeEvent writes the event field to the buffer if specified,
// escaping CR and LF characters. Defaults to "message" when absent on the client-side.
func (e *messageEncoder) writeEvent(event string) {
	if len(event) == 0 {
		return
	}

	e.buffer.Write(fieldPrefixEvent)
	e.buffer.WriteString(lineBreakReplacer.Replace(event))
	e.buffer.Write(byteLF)
}

// writeData formats and writes the data field to the buffer,
// handling multiline data by appending "data: " before each line and escaping CR characters.
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

// writeRetry writes the retry field if the value is non-zero, indicating
// the reconnection time clients should wait before reconnecting.
func (e *messageEncoder) writeRetry(retry int) {
	if retry == 0 {
		return
	}

	e.buffer.Write(fieldPrefixRetry)
	e.buffer.WriteString(strconv.Itoa(retry))
	e.buffer.Write(byteLF)
}

// encodeToBytes encodes the message into SSE wire format, ensuring each field is formatted as specified
// and appends an empty line to signal message termination. The buffer is resetBuffers post-write.
func (e *messageEncoder) encodeToBytes(msg *Message) []byte {
	defer e.buffer.Reset()

	e.writeID(msg.ID)
	e.writeEvent(msg.Event)
	e.writeData(msg.Data)
	e.writeRetry(msg.Retry)
	e.buffer.Write(byteLF) // Terminate the message

	return e.buffer.Bytes()
}

// Encode validates and encodes a message into the SSE wire format, ensuring all messages
// are valid. If not, returns an error indicating an invalid (empty) message.
func (e *messageEncoder) Encode(msg *Message) ([]byte, error) {
	if !e.isValidMessage(msg) {
		return nil, ErrMessageNoContent
	}

	return e.encodeToBytes(msg), nil
}

// messageDecoder processes SSE messages from an io.Reader, parsing fields and
// detecting message boundaries. Note: This decoder is single-threaded.
type messageDecoder struct {
	lastError      error          // Records any error encountered
	currentMessage Message        // Stores the most recent message
	reader         io.Reader      // Source stream containing SSE messages
	scanner        *bufio.Scanner // Line scanner for reading the input stream
	lastID         string         // Stores the last parsed ID
	eventBuffer    *bytes.Buffer  // Buffer for the current event field
	dataBuffer     *bytes.Buffer  // Buffer for the current data field
	retry          int            // Current retry value for reconnections
}

// newMessageDecoder initializes a new SSE decoder for a given Reader, typically the HTTP response body.
func newMessageDecoder(readCloser io.Reader) *messageDecoder {
	return &messageDecoder{
		reader:      readCloser,
		scanner:     bufio.NewScanner(readCloser),
		eventBuffer: bytes.NewBuffer(make([]byte, 0, 64)),
		dataBuffer:  bytes.NewBuffer(make([]byte, 0, 128)),
	}
}

// normalizeValue trims leading whitespace from a field value, resolves UTF-8 encoding issues,
// and replaces any invalid UTF-8 sequences with the Unicode replacement character.
func (e *messageDecoder) normalizeValue(value string) string {
	value = strings.TrimPrefix(value, whitespace)
	value = strings.TrimPrefix(value, utf8BomSequence)
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, invalidUTF8Replacement)
	}
	return value
}

// hasContent determines if the current message is ready to be dispatched.
// According to the SSE specification, a message should have at least one non-empty field.
// This method checks if either the eventBuffer or dataBuffer contains content.
// Returns true if there is content to constructMessage, false otherwise.
func (e *messageDecoder) hasContent() bool {
	if e.eventBuffer.Len() == 0 &&
		e.dataBuffer.Len() == 0 {
		return false
	}
	return true
}

// constructMessage constructs a Message from the accumulated field values and updates currentMessage.
// This is called when a complete SSE message has been parsed (indicated by an empty line).
// The method:
// - Extracts the event type from eventBuffer
// - Retrieves data content from dataBuffer, removing trailing newline
// - Sets the message ID from the lastID value (which persists across messages until changed)
// - Sets the retry value for reconnection timing
// - Resets all buffers after message construction by calling resetBuffers()
func (e *messageDecoder) constructMessage() {
	defer e.resetBuffers()

	event := e.eventBuffer.String()
	data := bytes.TrimSuffix(e.dataBuffer.Bytes(), byteLF)
	e.currentMessage.ID = e.lastID
	e.currentMessage.Event = event
	e.currentMessage.Data = data
	e.currentMessage.Retry = e.retry
}

// resetBuffers clears all temporary buffers and values after a message has been dispatched.
// This prepares the decoder for processing the next message by:
// - Clearing the event and data buffers
// - Resetting the ID (ID values are meant to persist between messages according to the spec)
// - Resetting the retry value to zero
// Note: According to the SSE specification, the lastID is not resetBuffers as it should
// persist until a new ID is received or the connection is closed.
func (e *messageDecoder) resetBuffers() {
	e.eventBuffer.Reset()
	e.dataBuffer.Reset()
	e.lastID = ""
	e.retry = 0
}

// processLine processes a single line of input from the SSE stream.
// It extracts the key-value pairs from the line according to the SSE specification,
// updating the internal state of the message decoder (eventBuffer, dataBuffer, lastID, retry).
// The method performs the following tasks:
// - Ignores comment lines that start with a colon (':')
// - If the line contains a field delimiter (':'), it separates the field name from the value.
// - It sanitizes the field value to remove leading whitespace and handle UTF-8 issues.
// - Updates the appropriate internal buffers or fields based on the field name:
//   - "id": Updates lastID with the provided value
//   - "event": Appends the value to the eventBuffer
//   - "data": Appends the value to the dataBuffer and adds a newline to adhere to multi-line data handling
//   - "retry": Converts the value to an integer and updates the retry value if it's positive
func (e *messageDecoder) processLine(line string) {
	if strings.HasPrefix(line, delimiter) {
		return // Ignore comment lines per specification
	}

	key, value, found := strings.Cut(line, delimiter)
	if !found {
		key = line
		value = ""
	} else {
		value = e.normalizeValue(value)
	}

	// Update internal state based on the field being processed
	switch key {
	case fieldID:
		e.lastID = value // Store the lastID for the current message
	case fieldEvent:
		e.eventBuffer.WriteString(value) // Collect event data
	case fieldData:
		e.dataBuffer.WriteString(value) // Collect data lines
		e.dataBuffer.Write(byteLF)      // Add a newline after each data line per spec
	case fieldRetry:
		retry, _ := strconv.Atoi(value) // Convert retry value to integer
		if retry > 0 {
			e.retry = retry // Update the retry value only if it's positive
		}
	}

}

// Current provides access to the most recent message parsed,
// intended for use after Next() returns true to retrieve the decoded message.
func (e *messageDecoder) Current() Message {
	return e.currentMessage
}

// Next retrieves the next message from the stream, parsing lines until a complete
// message is found (indicated by an empty line) or the stream ends. Returns true
// if a complete message was successfully parsed, false otherwise or if an error occurred.
func (e *messageDecoder) Next() bool {
	if e.lastError != nil {
		return false
	}

	var insideMessageBlock = false // Track message start

	for e.scanner.Scan() {
		line := e.scanner.Text()

		// Empty line marks end of message
		if len(line) == 0 {
			if !insideMessageBlock {
				continue // Skip initial empty lines
			}
			// Dispatch complete message
			e.constructMessage()
			return true
		}

		insideMessageBlock = true
		e.processLine(line)
	}

	// Check and constructMessage any final message
	if e.hasContent() {
		e.constructMessage()
		return true
	}

	// Detect errors or stream end
	e.lastError = e.scanner.Err()
	return false
}

// Error retrieves any error encountered during decoding.
// Intended to be checked after Next() returns false for determining normal
// end-of-stream vs. error termination.
func (e *messageDecoder) Error() error {
	return e.lastError
}
