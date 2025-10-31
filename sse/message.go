package sse

import (
	"errors"
	"strings"
	"unicode"
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
//   - ID: Uniquely identifies the event and enables connection resumption
//   - Event: Defines the event type (defaults to "message" if not specified)
//   - Data: Contains the event payload
//   - Retry: Specifies the reconnection time in milliseconds
type Message struct {
	ID    string // Message identifier
	Event string // Event type
	Data  []byte // Event payload
	Retry int    // Reconnection time in milliseconds
}

// isValidSSEEventName checks if the SSE event name meets the specification requirements.
// If the event name is empty, it's considered valid as the default "message" type will be used.
// Otherwise, it must follow DOM event naming rules.
//
// Valid event name rules:
//   - Empty string is valid (default "message" type will be used)
//   - Must start with a letter
//   - Can only contain letters, digits, underscore, hyphen, and period
//   - Cannot contain ".." sequence
//   - Cannot start or end with a period
//   - Cannot contain any whitespace characters
//
// Valid examples: "update", "user.created", "system-alert"
// Invalid examples: ".update", "user..profile", "alert!"
func isValidSSEEventName(eventName string) bool {
	if eventName == "" {
		return true
	}
	return isValidDOMEventName(eventName)
}

// isValidDOMEventName validates event names according to DOM specifications.
//
// Requirements:
//   - Must not be empty
//   - Must not contain ".." or start/end with "."
//   - Must start with a letter
//   - Can only contain letters, digits, underscore, hyphen, or period
//   - Cannot contain any whitespace
//
// Returns:
//   - true if the event name is valid according to DOM specifications
//   - false otherwise
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
