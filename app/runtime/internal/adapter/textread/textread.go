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
	if cause := context.Cause(ctx); cause != nil {
		return Result{}, cause
	}
	if source == nil || options.InputBytes <= 0 || options.LineBytes <= 0 || options.OutputBytes <= 0 {
		return Result{}, errors.New("textread: positive input, line, and output limits are required")
	}

	counter := &byteCounter{reader: contextReader{
		ctx:    ctx,
		reader: io.LimitReader(source, options.InputBytes+1),
	}}
	reader := bufio.NewReaderSize(counter, 64<<10)
	start := max(options.StartLine, 0)
	collector := lineCollector{
		start:       start,
		maxLines:    options.MaxLines,
		maxBytes:    options.OutputBytes,
		partialLine: options.PartialLine,
	}

	line := make([]byte, 0, 64<<10)
	endedWithNewline := false
	for {
		fragment, readErr := reader.ReadSlice('\n')
		rawLineLimit := options.LineBytes + 2
		if collector.total == 0 {
			rawLineLimit += 3
		}
		if len(line)+len(fragment) > rawLineLimit {
			return Result{}, &lineError{number: collector.total + 1, cause: ErrLineTooLarge}
		}
		line = append(line, fragment...)
		switch {
		case readErr == nil:
			line = normalizeLine(line[:len(line)-1], collector.total == 0)
			if err := collector.consume(line, options.LineBytes); err != nil {
				return Result{}, err
			}
			line = line[:0]
			endedWithNewline = true
		case errors.Is(readErr, bufio.ErrBufferFull):
			endedWithNewline = false
			continue
		case errors.Is(readErr, io.EOF):
			if counter.bytes > options.InputBytes {
				return Result{}, ErrInputTooLarge
			}
			if len(line) > 0 {
				line = normalizeLine(line, collector.total == 0)
				if err := collector.consume(line, options.LineBytes); err != nil {
					return Result{}, err
				}
			} else if collector.total == 0 || endedWithNewline {
				if err := collector.consume(nil, options.LineBytes); err != nil {
					return Result{}, err
				}
			}
			if cause := context.Cause(ctx); cause != nil {
				return Result{}, cause
			}
			return collector.result(), nil
		default:
			return Result{}, readErr
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

func (collector *lineCollector) consume(line []byte, maxLineBytes int) error {
	if len(line) > maxLineBytes {
		return &lineError{number: collector.total + 1, cause: ErrLineTooLarge}
	}
	if !utf8.Valid(line) || bytes.IndexByte(line, 0) >= 0 {
		return ErrInvalidText
	}
	index := collector.total
	collector.total++
	if index < collector.start || collector.clipped ||
		(collector.maxLines > 0 && collector.selected >= collector.maxLines) {
		return nil
	}
	if collector.append(line) {
		collector.selected++
	}
	return nil
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
