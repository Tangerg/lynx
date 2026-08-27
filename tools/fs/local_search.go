package fs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/bmatcuk/doublestar/v4"
)

// Default result caps applied when the caller leaves MaxResults at 0.
// The defaults prevent LLM context bloat without forcing the LLM to pass a
// cap on every call.
const (
	defaultGrepMaxResults = 250
	defaultGlobMaxResults = 100
)

func (l *LocalExecutor) Glob(ctx context.Context, in GlobInput) (GlobResponse, error) {
	if in.Pattern == "" {
		return GlobResponse{}, ErrEmptyPattern
	}
	root := l.rootDir(in.Root)
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultGlobMaxResults
	}
	options := []doublestar.GlobOption{
		doublestar.WithFilesOnly(),
		doublestar.WithNoFollow(),
		doublestar.WithFailOnIOErrors(),
	}
	if in.IgnoreCase {
		options = append(options, doublestar.WithCaseInsensitive())
	}

	var paths []string
	err := doublestar.GlobWalk(os.DirFS(root), filepath.ToSlash(in.Pattern), func(name string, _ fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		paths = append(paths, filepath.FromSlash(name))
		return nil
	}, options...)
	if err != nil {
		return GlobResponse{}, fmt.Errorf("fs.LocalExecutor.Glob: %w", err)
	}
	slices.Sort(paths)

	truncated := false
	if len(paths) > maxResults {
		paths = paths[:maxResults]
		truncated = true
	}
	return GlobResponse{Paths: paths, Truncated: truncated}, nil
}
