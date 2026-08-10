// Package attachment turns local files into the runtime-neutral attachment
// values accepted by the agent domain. It is a filesystem adapter: callers
// depend on its small Resolver surface, while runtime implementations receive
// only agent.Attachment values and never learn how the CLI found them.
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const (
	DefaultMaxBytes = 20 << 20
	DefaultLimit    = 50
	maxVisited      = 100_000
)

var (
	ErrNotRegular = errors.New("attachment is not a regular file")
	ErrTooLarge   = errors.New("attachment is too large")
)

// Resolver resolves explicit paths and searches the workspace for completion.
// Root is absolute but need not exist until a filesystem operation is requested,
// which keeps session construction side-effect free and easy to test.
type Resolver struct {
	root     string
	maxBytes int64
}

// Match is one workspace-relative file completion.
type Match struct {
	Path    string
	Detail  string
	Matched []int
	score   int
}

func New(root string) (*Resolver, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("attachment: workspace is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("attachment: resolve workspace: %w", err)
	}
	abs = filepath.Clean(abs)
	if canonical, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = canonical
	} else if !errors.Is(evalErr, os.ErrNotExist) {
		return nil, fmt.Errorf("attachment: resolve workspace symlinks: %w", evalErr)
	}
	return &Resolver{root: abs, maxBytes: DefaultMaxBytes}, nil
}

// Resolve validates and classifies one explicit path. Symlinks are resolved so
// identity and duplicate detection refer to the same underlying file.
func (r *Resolver) Resolve(ctx context.Context, input string) (agent.Attachment, error) {
	if err := context.Cause(ctx); err != nil {
		return agent.Attachment{}, err
	}
	canonical, info, header, err := r.inspect(input)
	if err != nil {
		return agent.Attachment{}, err
	}
	return r.project(canonical, info, header), nil
}

func (r *Resolver) inspect(input string) (string, fs.FileInfo, []byte, error) {
	path, err := r.absolute(input)
	if err != nil {
		return "", nil, nil, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, nil, fmt.Errorf("attachment: resolve %q: %w", input, err)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return "", nil, nil, fmt.Errorf("attachment: open %q: %w", input, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", nil, nil, fmt.Errorf("attachment: inspect %q: %w", input, err)
	}
	if err := validateAttachmentInfo(info, input, r.maxBytes); err != nil {
		return "", nil, nil, err
	}
	header, err := readAttachmentHeader(file, input)
	if err != nil {
		return "", nil, nil, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", nil, nil, fmt.Errorf("attachment: canonical path: %w", err)
	}
	return canonical, info, header, nil
}

func validateAttachmentInfo(info fs.FileInfo, input string, maxBytes int64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrNotRegular, input)
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("%w: %s is %s (limit %s)", ErrTooLarge, input, byteSize(info.Size()), byteSize(maxBytes))
	}
	return nil
}

func readAttachmentHeader(file io.Reader, input string) ([]byte, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("attachment: inspect content of %q: %w", input, err)
	}
	return header[:n], nil
}

func (r *Resolver) project(canonical string, info fs.FileInfo, header []byte) agent.Attachment {
	mimeType := classifyMIME(canonical, header)
	name := filepath.Base(canonical)
	if relative, ok := r.relative(canonical); ok {
		name = relative
	}
	digest := sha256.Sum256([]byte(canonical + "\x00" + strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	return agent.Attachment{
		ID: "att_" + hex.EncodeToString(digest[:8]), Kind: attachmentKind(mimeType), Name: filepath.ToSlash(name),
		Path: canonical, MimeType: mimeType, Size: info.Size(),
	}
}

func attachmentKind(mimeType string) agent.AttachmentKind {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return agent.AttachmentImage
	case isTextMIME(mimeType):
		return agent.AttachmentText
	default:
		return agent.AttachmentFile
	}
}

// Complete searches regular files below Root and returns fuzzy-ranked relative
// paths. It does not follow directory symlinks, skips dependency/VCS internals,
// and obeys cancellation during the walk.
func (r *Resolver) Complete(ctx context.Context, query string, limit int) ([]Match, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	search := completionSearch{
		ctx: ctx, root: r.root, maxBytes: r.maxBytes,
		query: filepath.ToSlash(strings.TrimSpace(query)), matches: make([]Match, 0, limit),
	}
	err := filepath.WalkDir(r.root, search.visit)
	if err != nil {
		return nil, fmt.Errorf("attachment: search workspace: %w", err)
	}
	slices.SortStableFunc(search.matches, func(a, b Match) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if len(a.Path) != len(b.Path) {
			return len(a.Path) - len(b.Path)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return slices.Clone(search.matches[:min(len(search.matches), limit)]), nil
}

type completionSearch struct {
	ctx      context.Context
	root     string
	query    string
	maxBytes int64
	visited  int
	matches  []Match
}

func (s *completionSearch) visit(path string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		return s.handleWalkError(path, entry, walkErr)
	}
	if err := context.Cause(s.ctx); err != nil {
		return err
	}
	if entry.IsDir() {
		return s.visitDirectory(path, entry)
	}
	return s.visitFile(path, entry)
}

func (s *completionSearch) handleWalkError(path string, entry os.DirEntry, walkErr error) error {
	if path != s.root && entry != nil && entry.IsDir() {
		return filepath.SkipDir
	}
	return walkErr
}

func (s *completionSearch) visitDirectory(path string, entry os.DirEntry) error {
	if path != s.root && ignoredDirectory(entry.Name()) {
		return filepath.SkipDir
	}
	return nil
}

func (s *completionSearch) visitFile(path string, entry os.DirEntry) error {
	s.visited++
	if s.visited > maxVisited {
		return filepath.SkipAll
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return nil
	}
	info, ok := completionFileInfo(entry, s.maxBytes)
	if !ok {
		return nil
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return err
	}
	relative = filepath.ToSlash(relative)
	score, at, matched := fuzzyPath(s.query, relative)
	if matched {
		s.matches = append(s.matches, Match{Path: relative, Detail: byteSize(info.Size()), Matched: at, score: score})
	}
	return nil
}

// completionFileInfo makes completion's best-effort policy explicit: a file
// that vanishes, becomes unreadable, is not regular, or exceeds the resolver's
// limit is simply not a candidate.
func completionFileInfo(entry fs.DirEntry, maxBytes int64) (fs.FileInfo, bool) {
	info, err := entry.Info()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, false
	}
	return info, true
}

func (r *Resolver) absolute(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("attachment: path is empty")
	}
	if input == "~" || strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("attachment: resolve home directory: %w", err)
		}
		if input == "~" {
			input = home
		} else {
			input = filepath.Join(home, strings.TrimPrefix(input, "~/"))
		}
	}
	if !filepath.IsAbs(input) {
		input = filepath.Join(r.root, input)
	}
	return filepath.Clean(input), nil
}

func (r *Resolver) relative(path string) (string, bool) {
	relative, err := filepath.Rel(r.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

func classifyMIME(path string, header []byte) string {
	if known := knownMIME[strings.ToLower(filepath.Ext(path))]; known != "" {
		return known
	}
	byExtension := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if byExtension != "" {
		if base, _, err := mime.ParseMediaType(byExtension); err == nil {
			return base
		}
	}
	detected := http.DetectContentType(header)
	if base, _, err := mime.ParseMediaType(detected); err == nil {
		return base
	}
	return detected
}

var knownMIME = map[string]string{
	".go": "text/x-go", ".md": "text/markdown", ".txt": "text/plain",
	".json": "application/json", ".yaml": "application/yaml", ".yml": "application/yaml",
	".toml": "application/toml", ".xml": "application/xml", ".csv": "text/csv",
	".js": "text/javascript", ".jsx": "text/jsx", ".ts": "text/typescript", ".tsx": "text/tsx",
	".py": "text/x-python", ".rs": "text/x-rust", ".sh": "text/x-shellscript",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".svg": "image/svg+xml",
}

func isTextMIME(value string) bool {
	return strings.HasPrefix(value, "text/") || strings.Contains(value, "json") ||
		strings.Contains(value, "xml") || strings.Contains(value, "yaml") ||
		strings.Contains(value, "javascript")
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", ".cache", "node_modules", "vendor":
		return true
	default:
		return false
	}
}

// fuzzyPath is a filesystem-local subsequence matcher. Keeping it here avoids
// making a non-terminal adapter depend on oolong merely to rank names.
func fuzzyPath(pattern, candidate string) (int, []int, bool) {
	if pattern == "" {
		return 0, nil, true
	}
	rest := pattern
	matched := make([]int, 0, utf8.RuneCountInString(pattern))
	score, previous := 0, -2
	prevRune := rune(-1)
	for offset, value := range candidate {
		if rest == "" {
			break
		}
		want, size := utf8.DecodeRuneInString(rest)
		if !runeEqualFold(value, want) {
			prevRune = value
			continue
		}
		score += 10
		if offset == 0 || strings.ContainsRune(" _-./:", prevRune) {
			score += 20
		}
		if previous >= 0 && offset == previous+utf8.RuneLen(prevRune) {
			score += 12
		}
		matched = append(matched, offset)
		previous = offset
		rest = rest[size:]
		prevRune = value
	}
	return score, matched, rest == ""
}

func runeEqualFold(a, b rune) bool {
	if a == b {
		return true
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}

func byteSize(value int64) string {
	const unit = 1024
	if value < unit {
		return strconv.FormatInt(value, 10) + " B"
	}
	divisor, suffix := int64(unit), "KiB"
	if value >= unit*unit*unit {
		divisor, suffix = unit*unit*unit, "GiB"
	} else if value >= unit*unit {
		divisor, suffix = unit*unit, "MiB"
	}
	return strconv.FormatFloat(float64(value)/float64(divisor), 'f', 1, 64) + " " + suffix
}
