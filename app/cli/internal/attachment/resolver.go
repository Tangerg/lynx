// Package attachment turns local files into the runtime-neutral attachment
// values accepted by the client domain. It is a filesystem adapter: callers
// depend on its small Resolver surface, while runtime implementations receive
// only client.Attachment values and never learn how the CLI found them.
package attachment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

const (
	DefaultMaxBytes = 20 << 20
	DefaultLimit    = 50
	MaxAttachments  = 16
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

// Root is the absolute workspace used for relative paths and completion.
func (r *Resolver) Root() string { return r.root }

// Resolve validates and classifies one explicit path. Symlinks are resolved so
// identity and duplicate detection refer to the same underlying file.
func (r *Resolver) Resolve(ctx context.Context, input string) (client.Attachment, error) {
	if err := context.Cause(ctx); err != nil {
		return client.Attachment{}, err
	}
	path, err := r.absolute(input)
	if err != nil {
		return client.Attachment{}, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return client.Attachment{}, fmt.Errorf("attachment: resolve %q: %w", input, err)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return client.Attachment{}, fmt.Errorf("attachment: open %q: %w", input, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return client.Attachment{}, fmt.Errorf("attachment: inspect %q: %w", input, err)
	}
	if !info.Mode().IsRegular() {
		return client.Attachment{}, fmt.Errorf("%w: %s", ErrNotRegular, input)
	}
	if info.Size() > r.maxBytes {
		return client.Attachment{}, fmt.Errorf("%w: %s is %s (limit %s)", ErrTooLarge, input, byteSize(info.Size()), byteSize(r.maxBytes))
	}

	header := make([]byte, 512)
	n, readErr := io.ReadFull(file, header)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return client.Attachment{}, fmt.Errorf("attachment: inspect content of %q: %w", input, readErr)
	}
	mimeType := classifyMIME(canonical, header[:n])
	kind := client.AttachmentFile
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		kind = client.AttachmentImage
	case isTextMIME(mimeType):
		kind = client.AttachmentText
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return client.Attachment{}, fmt.Errorf("attachment: canonical path: %w", err)
	}
	name := filepath.Base(canonical)
	if relative, ok := r.relative(canonical); ok {
		name = relative
	}
	digest := sha256.Sum256([]byte(canonical + "\x00" + strconv.FormatInt(info.Size(), 10) + "\x00" + strconv.FormatInt(info.ModTime().UnixNano(), 10)))
	return client.Attachment{
		ID: "att_" + hex.EncodeToString(digest[:8]), Kind: kind, Name: filepath.ToSlash(name),
		Path: canonical, MimeType: mimeType, Size: info.Size(),
	}, nil
}

// Complete searches regular files below Root and returns fuzzy-ranked relative
// paths. It does not follow directory symlinks, skips dependency/VCS internals,
// and obeys cancellation during the walk.
func (r *Resolver) Complete(ctx context.Context, query string, limit int) ([]Match, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	query = filepath.ToSlash(strings.TrimSpace(query))
	matches := make([]Match, 0, limit)
	visited := 0
	err := filepath.WalkDir(r.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path != r.root && entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return walkErr
		}
		if err := context.Cause(ctx); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != r.root && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		if visited > maxVisited {
			return filepath.SkipAll
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > r.maxBytes {
			return nil
		}
		relative, err := filepath.Rel(r.root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		score, at, ok := fuzzyPath(query, relative)
		if !ok {
			return nil
		}
		matches = append(matches, Match{
			Path: relative, Detail: byteSize(info.Size()), Matched: at, score: score,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("attachment: search workspace: %w", err)
	}
	slices.SortStableFunc(matches, func(a, b Match) int {
		if a.score != b.score {
			return b.score - a.score
		}
		if len(a.Path) != len(b.Path) {
			return len(a.Path) - len(b.Path)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return slices.Clone(matches[:min(len(matches), limit)]), nil
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
