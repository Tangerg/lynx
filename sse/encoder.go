package sse

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

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
//
// Returns:
//   - true if the message has at least one non-empty field (ID, Event, or Data)
//   - false if all fields are empty
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
//
// Parameters:
//   - messageID: The message ID to write
//   - outputBuffer: The buffer to write to
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
//
// Parameters:
//   - eventName: The event name to write
//   - outputBuffer: The buffer to write to
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
//
// Parameters:
//   - messageData: The data to write
//   - outputBuffer: The buffer to write to
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
//
// Parameters:
//   - retryValue: The retry time in milliseconds
//   - outputBuffer: The buffer to write to
func (e *Encoder) writeRetry(retryValue int, outputBuffer *bytes.Buffer) {
	if retryValue <= 0 {
		return
	}

	outputBuffer.Write(fieldPrefixRetry)
	outputBuffer.WriteString(strconv.Itoa(retryValue))
	outputBuffer.Write(byteLF)
}

// encodeToBytes formats the message into the SSE wire format according to the specification,
// ensuring each field is properly formatted and terminating the message with a blank line.
// This method is concurrency-safe as it creates a new buffer for each call.
//
// Parameters:
//   - messageToEncode: The message to encode
//
// Returns:
//   - The encoded message as a byte slice
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
// This method is concurrency-safe and can be called by multiple goroutines.
//
// Behavior:
//   - Empty string as Event is valid (clients will use default "message" type)
//   - Newlines in the Data field will be properly handled as multiline data fields
//   - Newlines in ID and Event fields will be escaped as \n
//   - Generated message will always end with a blank line
//   - If Retry value is zero or negative, it will be omitted from output
//
// Parameters:
//   - messageToEncode: The message to encode
//
// Returns:
//   - The encoded message as a byte slice
//   - An error if the message is invalid
//
// Errors:
//   - ErrMessageNoContent: If the message has no content (all fields are empty)
//   - ErrMessageInvalidEventName: If the event name doesn't follow DOM naming rules
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
