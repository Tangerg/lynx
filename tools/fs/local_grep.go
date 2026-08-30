package fs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ripgrepEventType string

const (
	ripgrepExecutable                         = "rg"
	ripgrepNoMatchesExitCode                  = 1
	ripgrepEventBegin        ripgrepEventType = "begin"
	ripgrepEventMatch        ripgrepEventType = "match"
	ripgrepEventContext      ripgrepEventType = "context"
	ripgrepEventEnd          ripgrepEventType = "end"
	ripgrepEventSummary      ripgrepEventType = "summary"
)

type ripgrepText struct {
	Text  *string `json:"text"`
	Bytes *string `json:"bytes"`
}

func (r ripgrepText) decode() (string, error) {
	if r.Text != nil && r.Bytes != nil {
		return "", errors.New("ripgrep text contains both text and bytes")
	}
	if r.Text != nil {
		return *r.Text, nil
	}
	if r.Bytes == nil {
		return "", errors.New("ripgrep text has no representation")
	}
	decoded, err := base64.StdEncoding.DecodeString(*r.Bytes)
	if err != nil {
		return "", fmt.Errorf("decode ripgrep bytes: %w", err)
	}
	return string(decoded), nil
}

type ripgrepEvent struct {
	Type ripgrepEventType `json:"type"`
	Data ripgrepEventData `json:"data"`
}

type ripgrepEventData struct {
	Path       ripgrepText `json:"path"`
	Lines      ripgrepText `json:"lines"`
	LineNumber uint64      `json:"line_number"`
}

type ripgrepDecoder struct {
	mode       GrepOutputMode
	maxResults int
	response   GrepResponse
	files      map[string]int
}

func newRipgrepDecoder(mode GrepOutputMode, maxResults int) *ripgrepDecoder {
	return &ripgrepDecoder{mode: mode, maxResults: maxResults, files: make(map[string]int)}
}

func (r *ripgrepDecoder) decode(reader io.Reader) (GrepResponse, error) {
	decoder := json.NewDecoder(reader)
	for {
		var event ripgrepEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return r.response, nil
			}
			return GrepResponse{}, fmt.Errorf("decode ripgrep event: %w", err)
		}
		if err := r.accept(event); err != nil {
			return GrepResponse{}, err
		}
	}
}

func (r *ripgrepDecoder) accept(event ripgrepEvent) error {
	switch event.Type {
	case ripgrepEventBegin, ripgrepEventEnd, ripgrepEventSummary:
		return nil
	case ripgrepEventMatch, ripgrepEventContext:
		return r.acceptLine(event)
	default:
		return fmt.Errorf("unsupported ripgrep event type %q", event.Type)
	}
}

func (r *ripgrepDecoder) acceptLine(event ripgrepEvent) error {
	path, err := event.Data.Path.decode()
	if err != nil {
		return fmt.Errorf("decode ripgrep path: %w", err)
	}
	if path == "" {
		return errors.New("ripgrep event contains an empty path")
	}
	path, err = normalizeRipgrepPath(path)
	if err != nil {
		return err
	}
	if event.Data.LineNumber == 0 || event.Data.LineNumber > uint64(math.MaxInt) {
		return fmt.Errorf("ripgrep event contains invalid line number %d", event.Data.LineNumber)
	}

	switch r.mode {
	case GrepOutputFilesWithMatches:
		if event.Type == ripgrepEventMatch {
			r.addFile(path)
		}
		return nil
	case GrepOutputCount:
		if event.Type == ripgrepEventMatch {
			r.addCount(path)
		}
		return nil
	}

	text, err := event.Data.Lines.decode()
	if err != nil {
		return fmt.Errorf("decode ripgrep line: %w", err)
	}
	kind := GrepLineContext
	if event.Type == ripgrepEventMatch {
		kind = GrepLineMatch
	}
	r.addLine(GrepLine{
		Path: path,
		Line: int(event.Data.LineNumber),
		Text: strings.TrimSuffix(strings.TrimSuffix(text, "\n"), "\r"),
		Kind: kind,
	})
	return nil
}

func normalizeRipgrepPath(value string) (string, error) {
	clean := filepath.Clean(value)
	if clean == "." || !filepath.IsLocal(clean) {
		return "", fmt.Errorf("ripgrep returned path outside executor root: %q", value)
	}
	return filepath.ToSlash(clean), nil
}

func (r *ripgrepDecoder) addLine(line GrepLine) {
	if len(r.response.Lines) < r.maxResults {
		r.response.Lines = append(r.response.Lines, line)
		return
	}
	r.response.Truncated = true
}

func (r *ripgrepDecoder) addFile(path string) {
	if _, exists := r.files[path]; exists {
		return
	}
	if len(r.response.Files) >= r.maxResults {
		r.response.Truncated = true
		return
	}
	r.files[path] = len(r.response.Files)
	r.response.Files = append(r.response.Files, path)
}

func (r *ripgrepDecoder) addCount(path string) {
	if index, exists := r.files[path]; exists {
		r.response.Counts[index].Count++
		return
	}
	if len(r.response.Counts) >= r.maxResults {
		r.response.Truncated = true
		return
	}
	r.files[path] = len(r.response.Counts)
	r.response.Counts = append(r.response.Counts, GrepFileCount{Path: path, Count: 1})
}

func (l *LocalExecutor) Grep(ctx context.Context, in GrepInput) (_ GrepResponse, err error) {
	if in.MaxResults < 0 || in.Context < 0 || in.BeforeContext < 0 || in.AfterContext < 0 ||
		in.Context > maximumContextLines || in.BeforeContext > maximumContextLines || in.AfterContext > maximumContextLines {
		return GrepResponse{}, fmt.Errorf("%w: grep result and context limits are outside their supported range", ErrInvalidInput)
	}
	if in.Pattern == "" {
		return GrepResponse{}, ErrEmptyPattern
	}
	if !in.OutputMode.Valid() {
		return GrepResponse{}, fmt.Errorf("fs.LocalExecutor.Grep: invalid output_mode %q", in.OutputMode)
	}
	base, err := l.authorize(in.Path, true)
	if err != nil {
		return GrepResponse{}, err
	}
	root, err := l.openRoot()
	if err != nil {
		return GrepResponse{}, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	info, err := root.Stat(base)
	if err != nil {
		return GrepResponse{}, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return GrepResponse{}, fmt.Errorf("fs.LocalExecutor.Grep: %s: unsupported file mode %s", in.Path, info.Mode().Type())
	}
	executable, err := exec.LookPath(ripgrepExecutable)
	if err != nil {
		return GrepResponse{}, fmt.Errorf("fs.LocalExecutor.Grep: %w: %w", ErrRipgrepUnavailable, err)
	}
	maxResults := in.MaxResults
	if maxResults == 0 {
		maxResults = defaultGrepMaxResults
	} else if maxResults > maximumSearchResults {
		return GrepResponse{}, fmt.Errorf("fs.LocalExecutor.Grep: max_results exceeds %d", maximumSearchResults)
	}
	mode := in.OutputMode.Resolve()
	args := in.ripgrepArguments(base, mode)
	response, err := runRipgrep(ctx, executable, args, newRipgrepDecoder(mode, maxResults), l.root)
	if err != nil {
		return GrepResponse{}, fmt.Errorf("fs.LocalExecutor.Grep: %w", err)
	}
	return response, nil
}

func (g GrepInput) ripgrepArguments(root string, mode GrepOutputMode) []string {
	args := []string{"--json", "--no-config", "--no-follow"}
	if mode == GrepOutputContent {
		before, after := g.contextLines()
		if before > 0 {
			args = append(args, "--before-context", strconv.Itoa(before))
		}
		if after > 0 {
			args = append(args, "--after-context", strconv.Itoa(after))
		}
	}
	if g.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if g.Multiline {
		args = append(args, "--multiline", "--multiline-dotall")
	}
	if g.FileType != "" {
		args = append(args, "--type", g.FileType)
	}
	if g.Glob != "" {
		args = append(args, "--glob", g.Glob)
	}
	return append(args, "--regexp", g.Pattern, "--", root)
}

func runRipgrep(
	ctx context.Context,
	path string,
	args []string,
	decoder *ripgrepDecoder,
	directory string,
) (GrepResponse, error) {
	command := exec.CommandContext(ctx, path, args...)
	command.Dir = directory
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return GrepResponse{}, fmt.Errorf("open ripgrep output: %w", err)
	}
	if err := command.Start(); err != nil {
		return GrepResponse{}, fmt.Errorf("start ripgrep: %w", err)
	}
	response, decodeErr := decoder.decode(stdout)
	if decodeErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return GrepResponse{}, decodeErr
	}
	waitErr := command.Wait()
	if waitErr == nil {
		return response, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return GrepResponse{}, ctxErr
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok && exitErr.ExitCode() == ripgrepNoMatchesExitCode {
		return response, nil
	}
	message := strings.TrimSpace(stderr.String())
	if message == "" {
		return GrepResponse{}, waitErr
	}
	return GrepResponse{}, fmt.Errorf("%w: %s", waitErr, message)
}
