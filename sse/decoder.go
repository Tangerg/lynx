package sse

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
	streamReader := bufio.NewReader(inputReader)

	decoder := &Decoder{
		streamReader: streamReader,
		lineScanner:  bufio.NewScanner(streamReader),
		dataBuffer:   bytes.NewBuffer(make([]byte, 0, 128)),
	}

	// Initialize decoder state
	decoder.skipLeadingUTF8BOM()
	decoder.lineScanner.Split(scanLinesSplit)

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

// scanLinesSplit copy form github.com/Tangerg/lynx/pkg/bufio.ScanLinesAllFormats
// implements a custom split function for bufio.Scanner to handle various
// line ending patterns in SSE streams. It properly processes:
// - CRLF (\r\n) sequences as a single line break
// - CR (\r) alone as a line break
// - LF (\n) alone as a line break
// - EOF at the end of data
// This ensures compatibility with different server implementations and platforms.
// The function returns the number of bytes to advance, the line token without
// line break characters, and any error encountered.
func scanLinesSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
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
