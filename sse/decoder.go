package sse

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// dropCR drops a terminal \r from the data.
func dropCR(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\r' {
		return data[0 : len(data)-1]
	}
	return data
}

// scanLinesSplit implements a custom split function for bufio.Scanner to handle various
// line ending patterns in SSE streams.
//
// Copied from github.com/Tangerg/lynx/pkg/bufio.ScanLinesAllFormats
func scanLinesSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// If we're at EOF and there's no data, return 0 to indicate no more tokens
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Find the position of the first '\n' character, returns -1 if not found
	n := bytes.IndexByte(data, '\n')
	// Find the position of the first '\r' character, returns -1 if not found
	r := bytes.IndexByte(data, '\r')

	// Handle the case when both \r and \n exist in the data
	if n >= 0 && r >= 0 {
		// Check if it's a Windows-style line ending "\r\n"
		if n == r+1 {
			// Advance past the '\n', drop the '\r' from the token
			return n + 1, dropCR(data[0:n]), nil
		}

		// For "\r...\n" or "\n...\r" patterns, use the earlier occurrence
		// min(n, r) gives us the position of whichever comes first
		i := min(n, r)
		// Advance past the first line ending character, drop any trailing '\r'
		return i + 1, dropCR(data[0:i]), nil
	}
	// Handle the case when only \r or only \n exists (not both)
	// max(n, r) returns the valid index (the other one is -1)
	if i := max(n, r); i >= 0 {
		// Advance past the line ending character, drop any trailing '\r'
		return i + 1, dropCR(data[0:i]), nil
	}

	// If we're at EOF, return the remaining data as the final line
	if atEOF {
		// No line ending characters exist in data at this point (both n and r are -1)
		// So no need to drop '\r'
		return len(data), data, nil
	}

	// Request more data by returning 0 advance with no token
	return 0, nil, nil
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
//
// The decoder initializes:
//   - A bufio.Reader that checks for and skips the UTF-8 Byte Order Mark (BOM) sequence
//   - A scanner with custom line splitting for SSE protocol
//   - Buffers for accumulating event type and data payloads
//   - Internal state for tracking message IDs and retry intervals
//
// The returned decoder is ready to parse SSE messages using the Next() method.
//
// Note: The decoder does not close the underlying reader when finished.
//
// Parameters:
//   - inputReader: The reader containing the SSE stream
//
// Returns:
//   - A new decoder ready to parse SSE messages
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
//
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

// normalizeValue processes a field value according to the SSE specification:
//   - Removes leading whitespace
//   - Handles UTF-8 BOM sequences
//   - Replaces invalid UTF-8 sequences with the replacement character
//
// Parameters:
//   - fieldValue: The field value to normalize
//
// Returns:
//   - The normalized field value
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
//
// Returns:
//   - true if there is meaningful data to be processed
//   - false otherwise
func (d *Decoder) hasValidData() bool {
	return d.dataBuffer.Len() > 1 // Buffer length > 1 indicates presence of actual data content
}

// dispatch constructs a message from accumulated field values if valid data is present.
// Also resets buffers after attempting to construct a message.
//
// Returns:
//   - true if a message was successfully constructed
//   - false otherwise
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

// constructMessage builds a Message from accumulated field values.
// Called when a complete SSE message has been parsed (indicated by an empty line).
//
// Processing steps:
//   - Removes trailing newline from data buffer
//   - Assigns persistent message ID from lastID
//   - Sets event type from eventBuffer
//   - Clones data to new memory allocation
//   - Sets retry interval for reconnection timing
func (d *Decoder) constructMessage() {
	// Remove trailing newline from data buffer
	messageData := bytes.TrimSuffix(d.dataBuffer.Bytes(), byteLF)

	// Build the message
	d.currentMessage.ID = d.lastID
	d.currentMessage.Event = d.eventBuffer
	d.currentMessage.Data = bytes.Clone(messageData) // Allocate new memory to avoid buffer reuse issues
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

// processLine handles a single line of input from the SSE stream according to the specification.
//
// Behavior:
//   - Ignores comment lines (starting with ':')
//   - Parses field name and value pairs
//   - Updates the appropriate internal state based on the field name:
//   - "id": Updates the lastID field
//   - "event": Sets the event buffer (defaults to "message" if empty)
//   - "data": Appends to the data buffer with a trailing newline
//   - "retry": Converts to an integer for reconnection timing (only if positive and valid)
//
// Parameters:
//   - inputLine: The line to process
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
//
// Returns:
//   - The current message
func (d *Decoder) Current() Message {
	return d.currentMessage
}

// Next advances to the next message in the stream, parsing lines until a complete
// message is found or the stream ends.
//
// Behavior:
//   - Returns false if the stream has ended
//   - Returns false and sets internal error state (retrievable via Error()) if an error is encountered
//   - Completes parsing of the current message and returns true when an empty line is encountered
//   - If an empty line is encountered but there is no current content, continues parsing until a message with content is found
//   - If there is an incomplete message at the end of the stream (not terminated by an empty line), still parses and returns that message
//   - If a message contains an invalid event name, that message will be ignored and parsing continues
//   - Invalid UTF-8 sequences are replaced with the U+FFFD character
//
// Returns:
//   - true if a message was successfully decoded
//   - false if the stream ended or an error occurred
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
//
// Returns:
//   - nil if the stream ended normally
//   - The error that caused decoding to stop
func (d *Decoder) Error() error {
	return d.lastError
}
