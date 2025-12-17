package xml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type interruptError struct {
	inner error
}

func (e *interruptError) Error() string {
	return e.inner.Error()
}
func (e *interruptError) Unwrap() error {
	return e.inner
}

// ElementListener defines a listener for specific XML elements.
// It provides callbacks when complete elements are parsed and allows buffer size control.
type ElementListener struct {
	Name          Name                // Element name to listen for
	OnComplete    func(Element) error // Callback when element parsing is complete
	MaxBufferSize int                 // Maximum buffer size for this element (bytes)
	EmitAlways    bool                // If true, emit nested elements immediately
}

// elementScope represents a parsing scope for an XML element.
// It tracks the element being built and its associated listener.
type elementScope struct {
	element  Element          // Element being constructed
	listener *ElementListener // Associated listener configuration
}

// appendCharData adds character data to the current scope's element.
// It intelligently merges consecutive CharData to optimize memory usage:
// 1. If no contents exist, append directly
// 2. If last content is CharData, merge with it
// 3. If last content is Element, append as new content
func (s *elementScope) appendCharData(chars CharData) {
	if len(s.element.Contents) == 0 {
		s.element.Contents = append(s.element.Contents, chars)
		return
	}

	lastContent := s.element.Contents[len(s.element.Contents)-1]
	switch typed := lastContent.(type) {
	case CharData:
		// Merge with existing CharData
		typed = append(typed, chars...)
		s.element.Contents[len(s.element.Contents)-1] = typed
	case Element:
		// Append as new content
		s.element.Contents = append(s.element.Contents, chars)
	}
}

// appendElement adds a complete element to the current scope.
func (s *elementScope) appendElement(element Element) {
	s.element.Contents = append(s.element.Contents, element)
}

// appendContent adds content (either CharData or Element) to the current scope.
func (s *elementScope) appendContent(content Content) {
	switch typed := content.(type) {
	case CharData:
		s.appendCharData(typed)
	case Element:
		s.appendElement(typed)
	}
}

// elementStack manages nested element scopes using a stack structure.
// Each scope has an associated buffer for accumulating the element's string representation.
type elementStack struct {
	length int             // Current stack depth
	scope  []*elementScope // Stack of element scopes
	buffer []*bytes.Buffer // Corresponding buffers for each scope
}

// newStack creates a new element stack with initial capacity.
func newStack() *elementStack {
	return &elementStack{
		scope:  make([]*elementScope, 0, 8),
		buffer: make([]*bytes.Buffer, 0, 8),
	}
}

// reset clears the stack to initial state.
func (s *elementStack) reset() {
	s.length = 0
	s.scope = s.scope[:0]
	s.buffer = s.buffer[:0]
}

// push adds a new scope and buffer to the stack.
func (s *elementStack) push(scope *elementScope, buffer *bytes.Buffer) {
	s.scope = append(s.scope, scope)
	s.buffer = append(s.buffer, buffer)
	s.length++
}

// pop removes and returns the top scope and buffer from the stack.
func (s *elementStack) pop() (*elementScope, *bytes.Buffer) {
	if s.length == 0 {
		return nil, nil
	}

	lastScope := s.scope[len(s.scope)-1]
	lastBuffer := s.buffer[len(s.buffer)-1]

	s.length--
	s.scope = s.scope[:s.length]
	s.buffer = s.buffer[:s.length]

	return lastScope, lastBuffer
}

// last returns the top scope and buffer without removing them.
func (s *elementStack) last() (*elementScope, *bytes.Buffer) {
	if s.length == 0 {
		return nil, nil
	}
	return s.scope[s.length-1], s.buffer[s.length-1]
}

// len returns the current stack depth.
func (s *elementStack) len() int {
	return s.length
}

// string returns the full stack bufferd element string
func (s *elementStack) string() string {
	sb := new(strings.Builder)
	for _, buffer := range s.buffer {
		sb.WriteString(buffer.String())
	}
	return sb.String()
}

// buffers holds various buffers used during parsing.
type buffers struct {
	element *bytes.Buffer // Buffer for current element being parsed
	text    *bytes.Buffer // Buffer for text content outside elements
}

// newBuffers creates a new set of buffers with specified initial size.
func newBuffers(size int) *buffers {
	return &buffers{
		element: bytes.NewBuffer(make([]byte, 0)),
		text:    bytes.NewBuffer(make([]byte, 0, size)),
	}
}

// reset clears all buffers to initial state.
func (b *buffers) reset() {
	b.element.Reset()
	b.text.Reset()
}

// elementState tracks the current parsing state.
type elementState struct {
	inElement bool // Inside an element (between < and >)
	inName    bool // Inside element name
	inAttrs   bool // Inside attribute section
	inString  bool // Inside a quoted string
	quoteChar byte // Current quote character (' or ")
}

// newElementState creates a new element state.
func newElementState() *elementState {
	return &elementState{}
}

// reset clears the state to initial values.
func (s *elementState) reset() {
	s.inElement = false
	s.inName = false
	s.inAttrs = false
	s.inString = false
	s.quoteChar = 0
}

// StreamScannerConfig configures the behavior of StreamScanner.
type StreamScannerConfig struct {
	Listeners       []*ElementListener // List of element listeners
	OnText          func(string) error // Callback for text content outside tracked elements
	MaxNestingLevel int                // Maximum allowed nesting depth
	BufferSize      int                // Buffer size for reading input
	StrictMode      bool               // If true, structural errors terminate parsing; if false, treat as text
	OnError         func(error)        // Optional error logging callback (called in non-strict mode)
}

// validate checks and normalizes the configuration.
func (c *StreamScannerConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if len(c.Listeners) == 0 {
		return errors.New("config.Listeners is empty")
	}

	// Set default values
	if c.MaxNestingLevel <= 0 {
		c.MaxNestingLevel = 256
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 4096
	}

	// Validate listeners
	eleNames := make(map[Name]bool)
	for _, listener := range c.Listeners {
		if listener.Name.String() == "" {
			return errors.New("element name is empty")
		}
		if eleNames[listener.Name] {
			return fmt.Errorf("element name %q is duplicated", listener.Name)
		}
		eleNames[listener.Name] = true

		if listener.MaxBufferSize <= 0 {
			listener.MaxBufferSize = 1024
		}
	}
	return nil
}

// StreamScanner performs streaming XML parsing with selective element tracking.
type StreamScanner struct {
	bufferSize      int                       // Size of read buffer
	maxNestingLevel int                       // Maximum nesting depth allowed
	listeners       map[Name]*ElementListener // Map of element listeners
	onText          func(string) error        // Text content callback
	strictMode      bool                      // Strict parsing mode
	onError         func(error)               // Error logging callback
	processedCount  int                       // Number of bytes processed
	stack           *elementStack             // Stack for nested elements
	buffers         *buffers                  // Parsing buffers
	elementState    *elementState             // Current parsing state
}

// NewStreamScanner creates a new stream scanner with the given configuration.
func NewStreamScanner(config *StreamScannerConfig) (*StreamScanner, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	scanner := &StreamScanner{
		bufferSize:      config.BufferSize,
		maxNestingLevel: config.MaxNestingLevel,
		listeners:       make(map[Name]*ElementListener),
		onText:          config.OnText,
		strictMode:      config.StrictMode,
		onError:         config.OnError,
		stack:           newStack(),
		buffers:         newBuffers(config.BufferSize),
		elementState:    newElementState(),
	}

	// Register listeners
	for _, listener := range config.Listeners {
		scanner.listeners[listener.Name] = listener
	}

	return scanner, nil
}

// Scan processes XML data from the reader.
func (p *StreamScanner) Scan(reader io.Reader) error {
	defer p.Reset()
	p.Reset()

	buf := make([]byte, p.bufferSize)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return p.finalize()
			}
			return fmt.Errorf("read error: %w", err)
		}
		if n > 0 {
			if processErr := p.processChunk(buf[:n]); processErr != nil {
				return processErr
			}
		}
	}
}

// Reset clears the scanner state for reuse.
func (p *StreamScanner) Reset() {
	p.stack.reset()
	p.buffers.reset()
	p.elementState.reset()
	p.processedCount = 0
}

// flushText sends accumulated text to the OnText callback.
func (p *StreamScanner) flushText() error {
	if p.buffers.text.Len() == 0 {
		return nil
	}

	if p.onText != nil {
		if err := p.onText(p.buffers.text.String()); err != nil {
			return &interruptError{
				inner: fmt.Errorf("OnText callback failed: %w", err),
			}
		}
	}

	p.buffers.text.Reset()
	return nil
}

// finalize completes the parsing process and validates final state.
func (p *StreamScanner) finalize() error {
	// Flush remaining text
	if err := p.flushText(); err != nil {
		return err
	}

	unClosed := p.stack.string()
	if p.strictMode && unClosed != "" {
		return fmt.Errorf("%w, unclosed element: %s", io.ErrUnexpectedEOF, unClosed)
	}

	p.buffers.text.WriteString(unClosed)

	return p.flushText()
}

// processChunk processes a chunk of data byte by byte.
func (p *StreamScanner) processChunk(data []byte) error {
	for _, b := range data {
		p.processedCount++
		if err := p.processByte(b); err != nil {
			if p.strictMode {
				return err
			}
			// if error is an interruptErr
			var interruptErr *interruptError
			if errors.As(err, &interruptErr) {
				return err
			}
			p.logError(err)
		}
	}
	return nil
}

// processByte processes a single byte based on current state.
func (p *StreamScanner) processByte(b byte) error {
	// Handle quote characters
	if (b == '"' || b == '\'') && p.elementState.inElement {
		if !p.elementState.inString {
			p.elementState.inString = true
			p.elementState.quoteChar = b
		} else if b == p.elementState.quoteChar {
			p.elementState.inString = false
			p.elementState.quoteChar = 0
		}
		p.buffers.element.WriteByte(b)
		return nil
	}

	// Handle quotes outside elements
	if (b == '"' || b == '\'') && !p.elementState.inElement {
		return p.writeToCurrentBuffer(b)
	}

	// If inside quoted string, write to element buffer
	if p.elementState.inString {
		p.buffers.element.WriteByte(b)
		return nil
	}

	// Handle whitespace
	if unicode.IsSpace(rune(b)) {
		return p.handleWhitespace(b)
	}

	// Handle special characters
	switch b {
	case '<':
		return p.handleElementOpen(b)
	case '>':
		return p.handleElementClose(b)
	default:
		return p.handleRegularChar(b)
	}
}

// isInScope returns true if currently inside a tracked element.
func (p *StreamScanner) isInScope() bool {
	return p.stack.len() > 0
}

// preCheckWriteBuffer check in advance whether the write exceeds the set capacity
func (p *StreamScanner) preCheckWriteBuffer(add int) error {
	if !p.isInScope() {
		return nil
	}
	lastScope, lastBuffer := p.stack.last()
	if lastScope == nil {
		return nil
	}
	listener, ok := p.listeners[lastScope.element.Start.Name]
	if !ok {
		return nil
	}
	if listener.MaxBufferSize > 0 && lastBuffer.Len()+add >= listener.MaxBufferSize {
		err := fmt.Errorf("overflow max buffer size is: %d, now buffer size is: %d", listener.MaxBufferSize, lastBuffer.Len()+add)
		if p.strictMode {
			return &interruptError{
				inner: err,
			}
		}
		return err
	}
	return nil
}

// writeToCurrentBuffer writes a byte to the appropriate buffer.
func (p *StreamScanner) writeToCurrentBuffer(b byte) error {
	err := p.preCheckWriteBuffer(1)
	if p.isInScope() {
		lastScope, lastBuffer := p.stack.last()
		lastScope.appendCharData(CharData{b})
		lastBuffer.WriteByte(b)
	} else {
		p.buffers.text.WriteByte(b)
	}
	return err
}

// writeStringToCurrentBuffer writes a string to the appropriate buffer.
func (p *StreamScanner) writeStringToCurrentBuffer(s string) error {
	err := p.preCheckWriteBuffer(len(s))
	if p.isInScope() {
		lastScope, lastBuffer := p.stack.last()
		lastScope.appendCharData(CharData(s))
		lastBuffer.WriteString(s)
	} else {
		p.buffers.text.WriteString(s)
	}
	return err
}

// handleWhitespace processes whitespace characters.
func (p *StreamScanner) handleWhitespace(b byte) error {
	if p.elementState.inElement {
		p.buffers.element.WriteByte(b)
		if p.elementState.inName {
			p.elementState.inName = false
			p.elementState.inAttrs = true
		}
		return nil
	}

	return p.writeToCurrentBuffer(b)
}

// handleElementOpen handles '<' character (element opening).
func (p *StreamScanner) handleElementOpen(b byte) error {
	if p.elementState.inElement {
		p.buffers.element.WriteByte(b)
		return nil
	}

	// Flush text content before starting new element
	if !p.isInScope() {
		if err := p.flushText(); err != nil {
			return err
		}
	}

	// Reset state for new element
	p.elementState.reset()
	p.elementState.inElement = true
	p.buffers.element.Reset()

	p.buffers.element.WriteByte(b)

	return nil
}

// handleElementClose handles '>' character (element closing).
func (p *StreamScanner) handleElementClose(b byte) error {
	if !p.elementState.inElement {
		return p.writeToCurrentBuffer(b)
	}

	p.buffers.element.WriteByte(b)

	// If inside quoted string, continue parsing
	if p.elementState.inString {
		return nil
	}

	eleContent := p.buffers.element.String()

	// Validate element syntax
	if !isValidElementSyntax(eleContent) {
		err := p.writeStringToCurrentBuffer(eleContent)
		if err != nil {
			return err
		}
		p.elementState.reset()
		return nil
	}

	p.elementState.reset()

	// Determine element type and process accordingly
	switch {
	case strings.HasPrefix(eleContent, "</"):
		return p.processCloseElement(eleContent)
	case strings.HasSuffix(eleContent, "/>"):
		return p.processSelfCloseElement(eleContent)
	default:
		return p.processOpenElement(eleContent)
	}
}

// handleRegularChar processes regular characters.
func (p *StreamScanner) handleRegularChar(b byte) error {
	if p.elementState.inElement {
		p.buffers.element.WriteByte(b)

		if !p.elementState.inName && !p.elementState.inAttrs && b != '/' {
			p.elementState.inName = true
			p.elementState.inAttrs = false
		}

		return nil
	}
	return p.writeToCurrentBuffer(b)
}

// logError logs an error in non-strict mode.
func (p *StreamScanner) logError(err error) {
	if p.onError != nil {
		p.onError(err)
	}
}

// processOpenElement processes an opening element like <element>.
func (p *StreamScanner) processOpenElement(eleContent string) error {
	eleName := extractElementName(eleContent)

	listener, isTracked := p.listeners[eleName]
	if !isTracked && !p.isInScope() {
		return p.writeStringToCurrentBuffer(eleContent)
	}

	// Check nesting level
	if p.stack.len() >= p.maxNestingLevel {
		err := errors.Join(
			p.writeStringToCurrentBuffer(eleContent),
			fmt.Errorf("nesting level %d exceeds maximum %d at position %d",
				p.stack.len()+1, p.maxNestingLevel, p.processedCount),
		)
		if p.strictMode {
			return &interruptError{
				inner: err,
			}
		}
		return err
	}

	// Parse attributes
	attrs, err := parseAttrs(eleContent)
	if err != nil {
		return errors.Join(
			p.writeStringToCurrentBuffer(eleContent),
			fmt.Errorf("failed to parse attributes for <%s> at position %d: %w",
				eleName, p.processedCount, err),
		)
	}

	return p.pushScope(eleName, attrs, eleContent, listener)
}

// processCloseElement processes a closing element like </element>.
func (p *StreamScanner) processCloseElement(eleContent string) error {
	eleName := extractElementName(eleContent)

	_, isTracked := p.listeners[eleName]
	if !isTracked && !p.isInScope() {
		return p.writeStringToCurrentBuffer(eleContent)
	}

	return p.popScope(eleName, eleContent)
}

// processSelfCloseElement processes a self-closing element like <element/>.
func (p *StreamScanner) processSelfCloseElement(eleContent string) error {
	eleName := extractElementName(eleContent)

	_, isTracked := p.listeners[eleName]
	if !isTracked && !p.isInScope() {
		return p.writeStringToCurrentBuffer(eleContent)
	}

	// Expand self-closing element into open and close elements
	openEle, closeEle := expandSelfCloseElement(eleContent, eleName.String())

	if err := p.processOpenElement(openEle); err != nil {
		return err
	}

	return p.processCloseElement(closeEle)
}

// pushScope pushes a new element scope onto the stack.
func (p *StreamScanner) pushScope(eleName Name, attrs []Attr, openEle string, listener *ElementListener) error {
	// Create scope buffer
	scopeBuffer := bytes.NewBuffer(make([]byte, 0, len(openEle)*2))
	scopeBuffer.WriteString(openEle)

	// Push new scope
	p.stack.push(&elementScope{
		element: Element{
			Start: StartElement{
				Name:  eleName,
				Attrs: attrs,
			},
		},
		listener: listener,
	}, scopeBuffer)

	return nil
}

// popScope pops an element scope from the stack and processes the complete element.
func (p *StreamScanner) popScope(eleName Name, closeEle string) error {
	currentScope, currentBuffer := p.stack.last()
	if currentScope == nil {
		p.buffers.text.WriteString(closeEle)
		return nil
	}

	// Stack not empty, check top element
	// Check if element name matches
	if currentScope.element.Start.Name != eleName {
		currentScope.appendCharData(CharData(closeEle))
		currentBuffer.WriteString(closeEle)
		err := fmt.Errorf("mismatched closing element at position %d: expected </%s>, got </%s>",
			p.processedCount, currentScope.element.Start.Name, eleName)
		return err
	}

	// Element name matches, pop from stack
	p.stack.pop()
	currentBuffer.WriteString(closeEle)
	fullEle := currentBuffer.String()

	// Build complete element
	element := Element{
		Start:    currentScope.element.Start,
		Contents: currentScope.element.Contents,
		End:      currentScope.element.Start.End(),
	}

	// Extract text content if no parsed contents
	if len(element.Contents) == 0 {
		textContent := extractElementContent(fullEle, currentScope.element.Start.Name.String())
		if len(textContent) > 0 {
			element.Contents = []Content{CharData(textContent)}
		}
	}

	// Add to parent scope or output buffer
	if p.stack.len() > 0 {
		lastScope, lastBuffer := p.stack.last()
		lastScope.appendElement(element)
		lastBuffer.WriteString(fullEle)
	}

	// Determine if callback should be triggered
	if currentScope.listener == nil {
		return nil
	}
	shouldEmit := currentScope.listener.EmitAlways || p.stack.len() == 0

	if !shouldEmit || currentScope.listener.OnComplete == nil {
		return nil
	}

	if err := currentScope.listener.OnComplete(element); err != nil {
		// External callback errors are always fatal
		return &interruptError{
			inner: fmt.Errorf("OnComplete callback failed for <%s>: %w",
				currentScope.element.Start.Name, err),
		}
	}

	return nil
}
