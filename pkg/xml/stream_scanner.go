package xml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type ElementListener struct {
	Name          Name
	OnComplete    func(Element) error
	MaxBufferSize int
	EmitAlways    bool
}

type scope struct {
	element  Element
	listener *ElementListener
}

type StreamScannerConfig struct {
	Listeners       []*ElementListener
	OnText          func(string) error
	MaxNestingLevel int
	BufferSize      int
}

func (c *StreamScannerConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if len(c.Listeners) == 0 {
		return errors.New("config.Listeners is empty")
	}

	if c.MaxNestingLevel <= 0 {
		c.MaxNestingLevel = 256
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 4096
	}

	tagNames := make(map[Name]bool)
	for _, listener := range c.Listeners {
		if listener.Name.String() == "" {
			return errors.New("tag name is empty")
		}
		if tagNames[listener.Name] {
			return fmt.Errorf("tag name %q is duplicated", listener.Name)
		}
		tagNames[listener.Name] = true

		if listener.MaxBufferSize <= 0 {
			listener.MaxBufferSize = 1 * 1024 * 1024
		}
	}

	return nil
}

type StreamScanner struct {
	bufferSize      int
	maxNestingLevel int
	listeners       map[Name]*ElementListener
	onText          func(string) error

	scopeStack  []*scope
	bufferStack []*bytes.Buffer

	outputBuffer  *bytes.Buffer
	tagBuffer     *bytes.Buffer
	tagNameBuffer *bytes.Buffer
	textBuffer    *bytes.Buffer

	insideTag      bool
	insideString   bool
	quoteChar      byte
	insideAttr     bool
	insideCloseTag bool
	processedCount int
}

func NewStreamScanner(config *StreamScannerConfig) (*StreamScanner, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	scanner := &StreamScanner{
		bufferSize:      config.BufferSize,
		maxNestingLevel: config.MaxNestingLevel,
		listeners:       make(map[Name]*ElementListener),
		onText:          config.OnText,

		scopeStack:  make([]*scope, 0, 8),
		bufferStack: make([]*bytes.Buffer, 0, 8),

		outputBuffer:  bytes.NewBuffer(make([]byte, 0, config.BufferSize)),
		tagBuffer:     bytes.NewBuffer(make([]byte, 0)),
		tagNameBuffer: bytes.NewBuffer(make([]byte, 0)),
		textBuffer:    bytes.NewBuffer(make([]byte, 0, config.BufferSize)),
	}

	for _, tag := range config.Listeners {
		scanner.listeners[tag.Name] = tag
	}

	return scanner, nil
}

func (p *StreamScanner) Scan(reader io.Reader) error {
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

func (p *StreamScanner) Reset() {
	p.scopeStack = p.scopeStack[:0]
	p.bufferStack = p.bufferStack[:0]

	p.outputBuffer.Reset()
	p.tagBuffer.Reset()
	p.tagNameBuffer.Reset()
	p.textBuffer.Reset()

	p.insideTag = false
	p.insideString = false
	p.quoteChar = 0
	p.insideAttr = false
	p.insideCloseTag = false
	p.processedCount = 0
}

func (p *StreamScanner) finalize() error {
	if err := p.flushText(); err != nil {
		return err
	}

	if err := p.flushOutput(); err != nil {
		return err
	}

	if p.insideTag {
		return fmt.Errorf("unexpected EOF at position %d: tag not closed, partial content: %q",
			p.processedCount, p.tagBuffer.String())
	}

	if len(p.scopeStack) > 0 {
		unclosed := p.scopeStack[len(p.scopeStack)-1].element.Start
		return fmt.Errorf("unexpected EOF at position %d: element <%s> not closed",
			p.processedCount, unclosed)
	}

	return nil
}

func (p *StreamScanner) processChunk(data []byte) error {
	for _, b := range data {
		p.processedCount++
		if err := p.processByte(b); err != nil {
			return err
		}
	}
	return nil
}

func (p *StreamScanner) processByte(b byte) error {
	if (b == '"' || b == '\'') && p.insideTag {
		if !p.insideString {
			p.insideString = true
			p.quoteChar = b
		} else if b == p.quoteChar {
			p.insideString = false
			p.quoteChar = 0
		}
		p.tagBuffer.WriteByte(b)
		return nil
	}

	if (b == '"' || b == '\'') && !p.insideTag {
		p.writeToCurrentBuffer(b)
		return nil
	}

	if p.insideString {
		p.tagBuffer.WriteByte(b)
		return nil
	}

	if unicode.IsSpace(rune(b)) {
		return p.handleWhitespace(b)
	}

	switch b {
	case '<':
		return p.handleTagOpen(b)
	case '>':
		return p.handleTagClose(b)
	default:
		return p.handleRegularChar(b)
	}
}

func (p *StreamScanner) isInScope() bool {
	return len(p.scopeStack) > 0
}

func (p *StreamScanner) writeToCurrentBuffer(b byte) {
	if p.isInScope() {
		p.bufferStack[len(p.bufferStack)-1].WriteByte(b)

		scope := p.scopeStack[len(p.scopeStack)-1]
		contents := scope.element.Contents

		if len(contents) > 0 {
			if chardata, ok := contents[len(contents)-1].(CharData); ok {
				scope.element.Contents[len(contents)-1] = append(chardata, b)
				return
			}
		}

		scope.element.Contents = append(scope.element.Contents, CharData{b})
	} else {
		p.textBuffer.WriteByte(b)
	}
}

func (p *StreamScanner) writeStringToCurrentBuffer(s string) {
	if p.isInScope() {
		p.bufferStack[len(p.bufferStack)-1].WriteString(s)

		scope := p.scopeStack[len(p.scopeStack)-1]
		contents := scope.element.Contents

		if len(contents) > 0 {
			if chardata, ok := contents[len(contents)-1].(CharData); ok {
				scope.element.Contents[len(contents)-1] = append(chardata, []byte(s)...)
				return
			}
		}

		scope.element.Contents = append(scope.element.Contents, CharData(s))
	} else {
		p.textBuffer.WriteString(s)
	}
}

func (p *StreamScanner) flushText() error {
	if p.textBuffer.Len() == 0 {
		return nil
	}

	if p.onText != nil {
		if err := p.onText(p.textBuffer.String()); err != nil {
			return fmt.Errorf("OnText callback failed: %w", err)
		}
	}

	p.textBuffer.Reset()
	return nil
}

func (p *StreamScanner) handleWhitespace(b byte) error {
	if p.insideTag {
		p.tagBuffer.WriteByte(b)
		if p.tagNameBuffer.Len() > 0 && !p.insideAttr {
			p.insideAttr = true
		}
	} else {
		p.writeToCurrentBuffer(b)
	}
	return nil
}

func (p *StreamScanner) handleTagOpen(b byte) error {
	if p.insideTag {
		p.tagBuffer.WriteByte(b)
		return nil
	}

	if !p.isInScope() {
		if err := p.flushText(); err != nil {
			return err
		}
	}

	p.insideTag = true
	p.insideAttr = false
	p.insideCloseTag = false

	p.tagNameBuffer.Reset()
	p.tagBuffer.Reset()
	p.tagBuffer.WriteByte(b)

	return nil
}

func (p *StreamScanner) handleTagClose(b byte) error {
	if !p.insideTag {
		p.writeToCurrentBuffer(b)
		return nil
	}

	p.tagBuffer.WriteByte(b)

	if p.insideString {
		return nil
	}

	tagContent := p.tagBuffer.String()

	if !p.isValidTagSyntax(tagContent) {
		p.writeStringToCurrentBuffer(tagContent)
		p.resetTagState()
		return nil
	}

	p.resetTagState()

	switch {
	case strings.HasPrefix(tagContent, "</"):
		return p.processCloseTag(tagContent)
	case strings.HasSuffix(tagContent, "/>"):
		return p.processSelfCloseTag(tagContent)
	default:
		return p.processOpenTag(tagContent)
	}
}

func (p *StreamScanner) handleRegularChar(b byte) error {
	if p.insideTag {
		p.tagBuffer.WriteByte(b)
		if !p.insideAttr && !p.insideCloseTag && b != '/' {
			p.tagNameBuffer.WriteByte(b)
		}
	} else {
		p.writeToCurrentBuffer(b)
	}
	return nil
}

func (p *StreamScanner) resetTagState() {
	p.insideTag = false
	p.insideAttr = false
	p.insideString = false
	p.quoteChar = 0
}

func (p *StreamScanner) processOpenTag(tagContent string) error {
	tagName := p.extractTagName(tagContent)

	listener, isTracked := p.listeners[tagName]
	if !isTracked {
		p.writeStringToCurrentBuffer(tagContent)
		return nil
	}

	if len(p.scopeStack) >= p.maxNestingLevel {
		return fmt.Errorf("nesting level %d exceeds maximum %d at position %d",
			len(p.scopeStack)+1, p.maxNestingLevel, p.processedCount)
	}

	attrs, err := p.parseAttrs(tagContent)
	if err != nil {
		return fmt.Errorf("failed to parse attributes for <%s> at position %d: %w",
			tagName, p.processedCount, err)
	}

	return p.pushScope(tagName, attrs, tagContent, listener)
}

func (p *StreamScanner) processCloseTag(tagContent string) error {
	tagName := p.extractTagName(tagContent)

	_, isTracked := p.listeners[tagName]
	if !isTracked {
		p.writeStringToCurrentBuffer(tagContent)
		return nil
	}

	return p.popScope(tagName, tagContent)
}

func (p *StreamScanner) processSelfCloseTag(tagContent string) error {
	tagName := p.extractTagName(tagContent)

	_, isTracked := p.listeners[tagName]
	if !isTracked {
		p.writeStringToCurrentBuffer(tagContent)
		return nil
	}

	openTag, closeTag := p.expandSelfCloseTag(tagName, tagContent)

	if err := p.processOpenTag(openTag); err != nil {
		return err
	}

	return p.processCloseTag(closeTag)
}

func (p *StreamScanner) pushScope(tagName Name, attrs []Attr, openTag string, listener *ElementListener) error {
	if listener.MaxBufferSize > 0 && p.outputBuffer.Len() > listener.MaxBufferSize {
		return fmt.Errorf("buffer size %d exceeds limit %d for <%s>",
			p.outputBuffer.Len(), listener.MaxBufferSize, tagName)
	}

	scopeBuffer := bytes.NewBuffer(make([]byte, 0, len(openTag)*2))
	scopeBuffer.WriteString(openTag)

	p.bufferStack = append(p.bufferStack, scopeBuffer)
	p.scopeStack = append(p.scopeStack, &scope{
		element: Element{
			Start: StartElement{
				Name:  tagName,
				Attrs: attrs,
			},
		},
		listener: listener,
	})

	return nil
}

func (p *StreamScanner) popScope(tagName Name, closeTag string) error {
	if len(p.scopeStack) == 0 {
		return fmt.Errorf("unexpected closing tag </%s> at position %d: no open tag",
			tagName, p.processedCount)
	}

	currentScope := p.scopeStack[len(p.scopeStack)-1]
	if currentScope.element.Start.Name != tagName {
		return fmt.Errorf("mismatched closing tag at position %d: expected </%s>, got </%s>",
			p.processedCount, currentScope.element.Start.Name, tagName)
	}

	p.scopeStack = p.scopeStack[:len(p.scopeStack)-1]
	currentBuffer := p.bufferStack[len(p.bufferStack)-1]
	p.bufferStack = p.bufferStack[:len(p.bufferStack)-1]

	currentBuffer.WriteString(closeTag)
	fullTag := currentBuffer.String()

	element := Element{
		Start:    currentScope.element.Start,
		Contents: currentScope.element.Contents,
		End:      currentScope.element.Start.End(),
	}

	if len(element.Contents) == 0 {
		textContent := p.extractTextContent(fullTag, currentScope.element.Start.Name.String())
		if len(textContent) > 0 {
			element.Contents = []Content{CharData(textContent)}
		}
	}

	shouldEmit := currentScope.listener.EmitAlways || len(p.scopeStack) == 0

	if shouldEmit && currentScope.listener.OnComplete != nil {
		if err := currentScope.listener.OnComplete(element); err != nil {
			return fmt.Errorf("OnComplete callback failed for <%s>: %w",
				currentScope.element.Start.Name, err)
		}
	}

	if len(p.bufferStack) > 0 {
		p.bufferStack[len(p.bufferStack)-1].WriteString(fullTag)

		parentScope := p.scopeStack[len(p.scopeStack)-1]
		parentScope.element.Contents = append(parentScope.element.Contents, element)
	} else {
		p.outputBuffer.WriteString(fullTag)
	}

	return nil
}

func (p *StreamScanner) flushOutput() error {
	if p.outputBuffer.Len() == 0 {
		return nil
	}

	p.outputBuffer.Reset()
	return nil
}

func (p *StreamScanner) isValidTagSyntax(tagContent string) bool {
	n := len(tagContent)
	if n < 3 || tagContent[0] != '<' || tagContent[n-1] != '>' {
		return false
	}

	start := 1
	if tagContent[start] == '/' {
		start++
	}

	for start < n-1 && unicode.IsSpace(rune(tagContent[start])) {
		start++
	}

	if start >= n-1 {
		return false
	}

	first := tagContent[start]
	if !unicode.IsLetter(rune(first)) && first != '_' {
		return false
	}

	for i := start + 1; i < n-1; i++ {
		c := rune(tagContent[i])
		if unicode.IsSpace(c) || c == '/' {
			break
		}
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' && c != '.' && c != ':' {
			return false
		}
	}

	return true
}

func (p *StreamScanner) extractTagName(tagContent string) Name {
	n := len(tagContent)
	start := 1
	if tagContent[start] == '/' {
		start++
	}

	for start < n && unicode.IsSpace(rune(tagContent[start])) {
		start++
	}

	end := start
	for end < n {
		c := tagContent[end]
		if unicode.IsSpace(rune(c)) || c == '>' || c == '/' {
			break
		}
		end++
	}
	return Name{Local: tagContent[start:end]}
}

func (p *StreamScanner) parseAttrs(openTag string) ([]Attr, error) {
	attrs := make([]Attr, 0, 4)

	if !strings.HasPrefix(openTag, "<") || !strings.HasSuffix(openTag, ">") {
		return nil, errors.New("invalid tag format")
	}

	inner := openTag[1 : len(openTag)-1]
	parts := strings.SplitN(inner, " ", 2)
	if len(parts) < 2 {
		return attrs, nil
	}

	attrStr := strings.TrimSpace(parts[1])
	pos := 0

	for pos < len(attrStr) {
		for pos < len(attrStr) && unicode.IsSpace(rune(attrStr[pos])) {
			pos++
		}
		if pos >= len(attrStr) {
			break
		}

		keyStart := pos
		for pos < len(attrStr) && attrStr[pos] != '=' && !unicode.IsSpace(rune(attrStr[pos])) {
			pos++
		}
		key := attrStr[keyStart:pos]

		for pos < len(attrStr) && (unicode.IsSpace(rune(attrStr[pos])) || attrStr[pos] == '=') {
			pos++
		}

		if pos >= len(attrStr) {
			return nil, fmt.Errorf("attribute %q has no value", key)
		}

		quote := attrStr[pos]
		if quote != '"' && quote != '\'' {
			return nil, fmt.Errorf("attribute %q value must be quoted", key)
		}
		pos++

		valueStart := pos

		for pos < len(attrStr) && attrStr[pos] != quote {
			pos++
		}

		if pos >= len(attrStr) {
			return nil, fmt.Errorf("attribute %q has unclosed quote", key)
		}

		value := attrStr[valueStart:pos]
		pos++

		attrs = append(attrs, Attr{
			Name:  Name{Local: key},
			Value: value,
		})
	}

	return attrs, nil
}

func (p *StreamScanner) extractTextContent(fullTag string, tagName string) []byte {
	openEnd := strings.Index(fullTag, ">")
	if openEnd == -1 {
		return nil
	}

	closeTag := "</" + tagName + ">"
	closeStart := strings.LastIndex(fullTag, closeTag)
	if closeStart == -1 || closeStart <= openEnd {
		return nil
	}

	return []byte(fullTag[openEnd+1 : closeStart])
}

func (p *StreamScanner) expandSelfCloseTag(tagName Name, tagContent string) (openTag, closeTag string) {
	open := strings.TrimSuffix(tagContent, "/>")
	open = strings.TrimRight(open, " \t\n\r")

	return open + ">", "</" + tagName.String() + ">"
}
