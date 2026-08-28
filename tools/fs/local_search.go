package fs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Default result caps applied when the caller leaves MaxResults at 0.
// The defaults prevent LLM context bloat without forcing the LLM to pass a
// cap on every call.
const (
	defaultGrepMaxResults = 250
	defaultGlobMaxResults = 100
	maximumSearchResults  = 1000
)

func (l *LocalExecutor) Glob(ctx context.Context, in GlobInput) (_ GlobResponse, err error) {
	if in.Pattern == "" {
		return GlobResponse{}, ErrEmptyPattern
	}
	if err := validateGlobPattern(in.Pattern); err != nil {
		return GlobResponse{}, err
	}
	base, err := l.authorize(in.Path, true)
	if err != nil {
		return GlobResponse{}, err
	}
	root, err := l.openRoot()
	if err != nil {
		return GlobResponse{}, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
	}()
	info, err := root.Stat(base)
	if err != nil {
		return GlobResponse{}, err
	}
	if !info.IsDir() {
		return GlobResponse{}, fmt.Errorf("fs.LocalExecutor.Glob: %s is not a directory", in.Path)
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultGlobMaxResults
	} else if maxResults > maximumSearchResults {
		return GlobResponse{}, fmt.Errorf("fs.LocalExecutor.Glob: max_results exceeds %d", maximumSearchResults)
	}
	options := []doublestar.GlobOption{
		doublestar.WithFilesOnly(),
		doublestar.WithNoFollow(),
		doublestar.WithFailOnIOErrors(),
	}
	if in.IgnoreCase {
		options = append(options, doublestar.WithCaseInsensitive())
	}

	var (
		paths     []string
		truncated bool
	)
	pattern := path.Join(filepath.ToSlash(base), filepath.ToSlash(in.Pattern))
	err = doublestar.GlobWalk(root.FS(), pattern, func(name string, _ fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		name = filepath.FromSlash(name)
		index, exists := slices.BinarySearch(paths, name)
		if exists {
			return nil
		}
		if len(paths) < maxResults {
			paths = slices.Insert(paths, index, name)
			return nil
		}
		truncated = true
		if index < maxResults {
			paths = slices.Insert(paths, index, name)
			paths = paths[:maxResults]
		}
		return nil
	}, options...)
	if err != nil {
		return GlobResponse{}, fmt.Errorf("fs.LocalExecutor.Glob: %w", err)
	}
	return GlobResponse{Paths: paths, Truncated: truncated}, nil
}

func validateGlobPattern(pattern string) error {
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("%w: glob pattern %q", ErrPathOutsideRoot, pattern)
	}
	for _, component := range strings.Split(filepath.ToSlash(pattern), "/") {
		if component == ".." {
			return fmt.Errorf("%w: glob pattern %q", ErrPathOutsideRoot, pattern)
		}
	}
	return nil
}
