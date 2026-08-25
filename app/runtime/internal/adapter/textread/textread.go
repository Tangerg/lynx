// Package textread provides Runtime-owned, bounded UTF-8 line scanning for
// filesystem adapters with different consumer result policies.
package textread

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

var (
	ErrInputTooLarge = errors.New("textread: input exceeds its byte limit")
	ErrLineTooLarge  = errors.New("textread: line exceeds its byte limit")
	ErrInvalidText   = errors.New("textread: input is not UTF-8 text")
)

type lineError struct {
	number int
	cause  error
}

func (err *lineError) Error() string { return err.cause.Error() }
func (err *lineError) Unwrap() error { return err.cause }

// LineNumber returns the one-based line attached to a scan failure, or zero
// when the failure is not line-specific.
func LineNumber(err error) int {
	var target *lineError
	if errors.As(err, &target) {
		return target.number
	}
	return 0
}

// Options defines one complete scan. StartLine is zero-based, MaxLines zero
// means the rest of the file, and PartialLine permits the last selected line
// to be clipped at a UTF-8 boundary when the output budget is exhausted.
type Options struct {
	InputBytes  int64
	LineBytes   int
	OutputBytes int
	StartLine   int
	MaxLines    int
	PartialLine bool
}

// Limits define one complete UTF-8 line scan without prescribing what a
// consumer retains. InputBytes includes line separators and a UTF-8 BOM;
// LineBytes applies after BOM/CRLF normalization.
type Limits struct {
	InputBytes int64
	LineBytes  int
}

// Result reports normalized LF text and the whole-file line count. StartLine
// is zero-based and EndLine is exclusive.
type Result struct {
	Content         string
	StartLine       int
	EndLine         int
	TotalLines      int
	Truncated       bool
	OutputTruncated bool
}

// Scan validates and counts the complete admitted input while retaining only
// the selected output window.
func Scan(ctx context.Context, source io.Reader, options Options) (Result, error) {
	if options.OutputBytes <= 0 {
		return Result{}, errors.New("textread: positive input, line, and output limits are required")
	}
	start := max(options.StartLine, 0)
	collector := lineCollector{
		start:       start,
		maxLines:    options.MaxLines,
		maxBytes:    options.OutputBytes,
		partialLine: options.PartialLine,
	}
	err := VisitLines(ctx, source, Limits{InputBytes: options.InputBytes, LineBytes: options.LineBytes},
		func(_ int, line []byte) error {
			collector.consume(line)
			return nil
		})
	if err != nil {
		return Result{}, err
	}
	return collector.result(), nil
}

// VisitLines validates and normalizes a complete bounded input, then calls
// visit for each one-based line. The line slice is valid only until visit
// returns. A trailing newline contributes a final empty line, matching Scan's
// editor-facing line identity.
func VisitLines(ctx context.Context, source io.Reader, limits Limits, visit func(number int, line []byte) error) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if source == nil || limits.InputBytes <= 0 || limits.LineBytes <= 0 || visit == nil {
		return errors.New("textread: positive input and line limits plus a visitor are required")
	}

	counter := &byteCounter{reader: contextReader{
		ctx:    ctx,
		reader: io.LimitReader(source, limits.InputBytes+1),
	}}
	reader := bufio.NewReaderSize(counter, 64<<10)
	line := make([]byte, 0, 64<<10)
	lineNumber := 0
	endedWithNewline := false
	consume := func(line []byte) error {
		lineNumber++
		if len(line) > limits.LineBytes {
			return &lineError{number: lineNumber, cause: ErrLineTooLarge}
		}
		if !utf8.Valid(line) || bytes.IndexByte(line, 0) >= 0 {
			return ErrInvalidText
		}
		return visit(lineNumber, line)
	}
	for {
		fragment, readErr := reader.ReadSlice('\n')
		rawLineLimit := limits.LineBytes + 2
		if lineNumber == 0 {
			rawLineLimit += 3
		}
		if len(line)+len(fragment) > rawLineLimit {
			return &lineError{number: lineNumber + 1, cause: ErrLineTooLarge}
		}
		line = append(line, fragment...)
		switch {
		case readErr == nil:
			line = normalizeLine(line[:len(line)-1], lineNumber == 0)
			if err := consume(line); err != nil {
				return err
			}
			line = line[:0]
			endedWithNewline = true
		case errors.Is(readErr, bufio.ErrBufferFull):
			endedWithNewline = false
			continue
		case errors.Is(readErr, io.EOF):
			if counter.bytes > limits.InputBytes {
				return ErrInputTooLarge
			}
			if len(line) > 0 {
				line = normalizeLine(line, lineNumber == 0)
				if err := consume(line); err != nil {
					return err
				}
			} else if lineNumber == 0 || endedWithNewline {
				if err := consume(nil); err != nil {
					return err
				}
			}
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			return nil
		default:
			return readErr
		}
	}
}

type lineCollector struct {
	content     strings.Builder
	start       int
	maxLines    int
	maxBytes    int
	partialLine bool
	total       int
	selected    int
	clipped     bool
}

func (collector *lineCollector) consume(line []byte) {
	index := collector.total
	collector.total++
	if index < collector.start || collector.clipped ||
		(collector.maxLines > 0 && collector.selected >= collector.maxLines) {
		return
	}
	if collector.append(line) {
		collector.selected++
	}
}

func (collector *lineCollector) append(line []byte) bool {
	separator := collector.selected > 0
	remaining := collector.maxBytes - collector.content.Len()
	if separator {
		remaining--
	}
	if remaining < 0 {
		collector.clipped = true
		return false
	}
	if len(line) <= remaining {
		if separator {
			collector.content.WriteByte('\n')
		}
		collector.content.Write(line)
		return true
	}
	if !collector.partialLine {
		collector.clipped = true
		return false
	}
	prefix := validPrefix(line, remaining)
	if prefix == 0 {
		collector.clipped = true
		return false
	}
	if separator {
		collector.content.WriteByte('\n')
	}
	collector.content.Write(line[:prefix])
	collector.clipped = true
	return true
}

func (collector *lineCollector) result() Result {
	start := min(collector.start, collector.total)
	end := start + collector.selected
	return Result{
		Content: collector.content.String(), StartLine: start, EndLine: end, TotalLines: collector.total,
		Truncated: start > 0 || end < collector.total || collector.clipped, OutputTruncated: collector.clipped,
	}
}

func normalizeLine(line []byte, first bool) []byte {
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	if first && len(line) >= 3 && line[0] == 0xef && line[1] == 0xbb && line[2] == 0xbf {
		line = line[3:]
	}
	return line
}

func validPrefix(line []byte, limit int) int {
	prefix := min(len(line), max(limit, 0))
	for prefix > 0 && !utf8.Valid(line[:prefix]) {
		prefix--
	}
	return prefix
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(reader.ctx); cause != nil {
		return 0, cause
	}
	read, err := reader.reader.Read(buffer)
	if cause := context.Cause(reader.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

type byteCounter struct {
	reader io.Reader
	bytes  int64
}

func (reader *byteCounter) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	reader.bytes += int64(read)
	return read, err
}
