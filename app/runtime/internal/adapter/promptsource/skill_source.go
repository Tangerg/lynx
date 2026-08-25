package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	sdk "github.com/Tangerg/lynx/skills"

	domainskills "github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// runtimeSkillSource is Runtime's finite admission boundary around the Agent
// Skills SDK. The SDK remains the format/resource implementation; Runtime owns
// the complete-list, document, and model-resource contract required by its
// model and UI consumers.
type runtimeSkillSource struct {
	root      string
	resources sdk.ResourceSource
}

func newRuntimeSkillSource(root string) sdk.ResourceSource {
	return &runtimeSkillSource{root: root, resources: sdk.Dir(root)}
}

func (s *runtimeSkillSource) List(ctx context.Context) ([]sdk.Summary, error) {
	entries, err := s.directoryEntries(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, min(len(entries), domainskills.MaxSkillsPerSource))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !validRuntimeSkillName(name) {
			continue
		}
		if len(names) == domainskills.MaxSkillsPerSource {
			return nil, fmt.Errorf(
				"%w: source %q contains more than %d Skills",
				domainskills.ErrLibraryCapacity,
				s.root,
				domainskills.MaxSkillsPerSource,
			)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	summaries := make([]sdk.Summary, 0, len(names))
	for _, name := range names {
		if err := skillSourceContextError(ctx, "list"); err != nil {
			return nil, err
		}
		skill, err := s.Load(ctx, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, sdk.ErrInvalidSkill) {
				continue
			}
			return nil, fmt.Errorf("runtime skill source: list: %w", err)
		}
		summaries = append(summaries, skill.Summary())
	}
	return summaries, nil
}

func (s *runtimeSkillSource) Load(ctx context.Context, name string) (*sdk.Skill, error) {
	if !validRuntimeSkillName(name) {
		return nil, fmt.Errorf("%w %q: invalid name", sdk.ErrInvalidSkill, name)
	}
	if err := skillSourceContextError(ctx, "load"); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", s.root, err)
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(filepath.Join(name, sdk.SkillFile))
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", name, err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(
			fmt.Errorf("runtime skill source: inspect %q: %w", name, statErr),
			file.Close(),
		)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(
			fmt.Errorf("runtime skill source: %q is not a regular document", name),
			file.Close(),
		)
	}
	if info.Size() > domainskills.MaxAuthoredSkillDocumentBytes {
		return nil, errors.Join(
			fmt.Errorf(
				"%w: %q is %d bytes; limit is %d",
				domainskills.ErrDocumentTooLarge,
				name,
				info.Size(),
				domainskills.MaxAuthoredSkillDocumentBytes,
			),
			file.Close(),
		)
	}
	content, readErr := io.ReadAll(io.LimitReader(
		skillSourceContextReader{ctx: ctx, reader: file},
		domainskills.MaxAuthoredSkillDocumentBytes+1,
	))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("runtime skill source: read %q: %w", name, errors.Join(readErr, closeErr))
	}
	if len(content) > domainskills.MaxAuthoredSkillDocumentBytes {
		return nil, fmt.Errorf(
			"%w: %q exceeds %d bytes",
			domainskills.ErrDocumentTooLarge,
			name,
			domainskills.MaxAuthoredSkillDocumentBytes,
		)
	}
	frontmatter, body, err := sdk.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", sdk.ErrInvalidSkill, name, err)
	}
	if err := frontmatter.Validate(); err != nil {
		return nil, fmt.Errorf("%w %q: %w", sdk.ErrInvalidSkill, name, err)
	}
	if frontmatter.Name != name {
		return nil, fmt.Errorf("%w %q: %w", sdk.ErrInvalidSkill, name, sdk.ErrNameMismatch)
	}
	return &sdk.Skill{Frontmatter: frontmatter, Body: body}, nil
}

func (s *runtimeSkillSource) OpenResource(ctx context.Context, name, resource string) (fs.File, error) {
	file, err := s.resources.OpenResource(ctx, name, resource)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("runtime skill source: inspect resource %q/%q: %w", name, resource, err), file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(
			fmt.Errorf("runtime skill source: resource %q/%q is not a regular file", name, resource),
			file.Close(),
		)
	}
	if info.Size() > domainskills.MaxSkillResourceBytes {
		return nil, errors.Join(
			fmt.Errorf(
				"%w: resource %q/%q is %d bytes; limit is %d",
				domainskills.ErrResourceTooLarge,
				name,
				resource,
				info.Size(),
				domainskills.MaxSkillResourceBytes,
			),
			file.Close(),
		)
	}
	if err := skillSourceContextError(ctx, "open resource"); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return &boundedSkillResource{File: file, ctx: ctx, name: name, resource: resource}, nil
}

func (s *runtimeSkillSource) directoryEntries(ctx context.Context) ([]fs.DirEntry, error) {
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runtime skill source: open %q: %w", s.root, err)
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	entries, readErr := directory.ReadDir(domainskills.MaxSkillDirectoryEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := errors.Join(directory.Close(), root.Close())
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("runtime skill source: list %q: %w", s.root, errors.Join(readErr, closeErr))
	}
	if len(entries) > domainskills.MaxSkillDirectoryEntries {
		return nil, fmt.Errorf(
			"%w: source %q contains more than %d directory entries",
			domainskills.ErrLibraryCapacity,
			s.root,
			domainskills.MaxSkillDirectoryEntries,
		)
	}
	if err := skillSourceContextError(ctx, "list"); err != nil {
		return nil, err
	}
	return entries, nil
}

func validRuntimeSkillName(name string) bool {
	return (sdk.Frontmatter{Name: name, Description: "Runtime Skill source entry"}).Validate() == nil
}

func skillSourceContextError(ctx context.Context, operation string) error {
	if cause := context.Cause(ctx); cause != nil {
		return fmt.Errorf("runtime skill source: %s: %w", operation, cause)
	}
	return nil
}

type skillSourceContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r skillSourceContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(r.ctx); cause != nil {
		return 0, cause
	}
	read, err := r.reader.Read(buffer)
	if cause := context.Cause(r.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

type boundedSkillResource struct {
	fs.File
	ctx      context.Context
	name     string
	resource string
	read     int64
}

func (f *boundedSkillResource) Read(buffer []byte) (int, error) {
	if cause := context.Cause(f.ctx); cause != nil {
		return 0, cause
	}
	remaining := int64(domainskills.MaxSkillResourceBytes) - f.read
	if remaining < 0 {
		return 0, f.tooLarge()
	}
	limit := int64(len(buffer))
	if limit > remaining+1 {
		limit = remaining + 1
	}
	read, err := f.File.Read(buffer[:limit])
	if cause := context.Cause(f.ctx); cause != nil {
		f.read += int64(read)
		return read, cause
	}
	if int64(read) <= remaining {
		f.read += int64(read)
		return read, err
	}
	allowed := int(remaining)
	f.read += int64(read)
	return allowed, f.tooLarge()
}

func (f *boundedSkillResource) tooLarge() error {
	return fmt.Errorf(
		"%w: resource %q/%q exceeds %d bytes",
		domainskills.ErrResourceTooLarge,
		f.name,
		f.resource,
		domainskills.MaxSkillResourceBytes,
	)
}
