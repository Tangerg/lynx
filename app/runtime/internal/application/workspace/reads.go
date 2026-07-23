package workspace

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

var (
	ErrFileListTooLarge = errors.New("workspace: file listing too large")
	ErrPageLimit        = errors.New("workspace: page limit invalid")
	ErrPageCursor       = errors.New("workspace: page cursor invalid")
	ErrVCSUnavailable   = errors.New("workspace: VCS unavailable")
	ErrVCSBaseUnknown   = errors.New("workspace: VCS base unknown")
)

// FileEntryKind identifies a file-system entry in the workspace browser.
type FileEntryKind string

const (
	FileEntryFile    FileEntryKind = "file"
	FileEntryDir     FileEntryKind = "dir"
	FileEntrySymlink FileEntryKind = "symlink"
)

// FileEntry is an inspected, root-relative workspace entry.
type FileEntry struct {
	Path       string
	Name       string
	Kind       FileEntryKind
	SizeBytes  int64
	ModifiedAt time.Time
}

func (e FileEntry) orderKey() string {
	class := "1"
	if e.Kind == FileEntryDir {
		class = "0"
	}
	return class + ":" + e.Path
}

// FileListOptions controls one workspace file listing.
type FileListOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

// FileBrowser is the application-owned port for workspace file reads.
type FileBrowser interface {
	ListFiles(ctx context.Context, root string, options FileListOptions) ([]FileEntry, error)
	ReadFile(ctx context.Context, root string, input FileReadInput) (FileReadResult, error)
	Grep(ctx context.Context, root string, input GrepInput) (GrepResult, error)
}

// FileListInput is one paged workspace listing request.
type FileListInput struct {
	Cwd string
	FileListOptions
	Cursor string
	Limit  int
}

// FilePage is a stable cursor page of workspace entries.
type FilePage struct {
	Entries    []FileEntry
	NextCursor string
}

// FileReadInput specifies a root-relative file read. StartLine/EndLine are
// one-based and inclusive when StartLine is positive.
type FileReadInput struct {
	Path      string
	MaxBytes  int
	StartLine int
	EndLine   int
}

// FileReadResult is a file read with whole-file line information.
type FileReadResult struct {
	Content    string
	TotalLines int
	StartLine  int // zero-based
	EndLine    int // one-based inclusive
	Truncated  bool
}

// GrepInput specifies one root-scoped content search.
type GrepInput struct {
	Path  string
	Query string
	Limit int
}

// GrepMatch is one content-search match.
type GrepMatch struct {
	Path       string
	LineNumber int
	Text       string
}

// GrepResult is a capped content search plus its honest total match count.
type GrepResult struct {
	Matches []GrepMatch
	Total   int
}

// FileHead is the leading numbered text of one file.
type FileHead struct {
	Lines []FileLine
}

// FileLine is one one-based line in a file preview.
type FileLine struct {
	Number int
	Text   string
}

const (
	defaultFileListPageLimit = 1000
	defaultFileHeadLines     = 200
	defaultGrepLimit         = 100
)

// ListFiles returns one stable cursor page of entries below a workspace root.
func (c *Files) ListFiles(ctx context.Context, input FileListInput) (FilePage, error) {
	root, err := c.context.root(input.Cwd)
	if err != nil {
		return FilePage{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = c.context.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return FilePage{}, err
		}
	}
	if c.files == nil {
		return FilePage{}, errors.New("workspace: file browser is not configured")
	}
	entries, err := c.files.ListFiles(ctx, root, FileListOptions{
		Path: path, Glob: input.Glob, Recursive: input.Recursive, IncludeIgnored: input.IncludeIgnored,
	})
	if err != nil {
		return FilePage{}, err
	}
	page, next, err := pageFileEntries(entries, input.Cursor, input.Limit)
	if err != nil {
		return FilePage{}, err
	}
	return FilePage{Entries: page, NextCursor: next}, nil
}

// FileHead returns the first requested lines of one workspace file.
func (c *Files) FileHead(ctx context.Context, cwd, path string, lines int) (FileHead, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return FileHead{}, err
	}
	if path == "" {
		return FileHead{}, ErrPathRequired
	}
	path, err = c.context.paths.ResolveExistingInRoot(root, path)
	if err != nil {
		return FileHead{}, err
	}
	if lines <= 0 {
		lines = defaultFileHeadLines
	}
	if c.files == nil {
		return FileHead{}, errors.New("workspace: file browser is not configured")
	}
	read, err := c.files.ReadFile(ctx, root, FileReadInput{Path: path, EndLine: lines, StartLine: 1})
	if err != nil {
		return FileHead{}, err
	}
	return FileHead{Lines: previewLines(read)}, nil
}

// ReadFile returns all or a one-based inclusive line window of a workspace
// file. It validates ranges before asking the filesystem adapter to read.
func (c *Files) ReadFile(ctx context.Context, cwd string, input FileReadInput) (FileReadResult, error) {
	if err := input.validate(); err != nil {
		return FileReadResult{}, err
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return FileReadResult{}, err
	}
	input.Path, err = c.context.paths.ResolveExistingInRoot(root, input.Path)
	if err != nil {
		return FileReadResult{}, err
	}
	if c.files == nil {
		return FileReadResult{}, errors.New("workspace: file browser is not configured")
	}
	return c.files.ReadFile(ctx, root, input)
}

func (input FileReadInput) validate() error {
	if input.Path == "" {
		return ErrPathRequired
	}
	if input.StartLine < 0 || input.EndLine < 0 || input.MaxBytes < 0 {
		return ErrInvalidFileRange
	}
	if input.EndLine > 0 && input.StartLine == 0 {
		return ErrInvalidFileRange
	}
	if input.StartLine > 0 && input.EndLine > 0 && input.EndLine < input.StartLine {
		return ErrInvalidFileRange
	}
	return nil
}

// Grep searches a workspace root or an existing subdirectory. A truncated
// search returns an honest total rather than silently under-reporting hits.
func (c *Files) Grep(ctx context.Context, cwd string, input GrepInput) (GrepResult, error) {
	if input.Query == "" {
		return GrepResult{}, ErrGrepQueryMissing
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return GrepResult{}, err
	}
	if input.Path != "" {
		input.Path, err = c.context.paths.ResolveExistingInRoot(root, input.Path)
		if err != nil {
			return GrepResult{}, err
		}
	}
	if input.Limit <= 0 {
		input.Limit = defaultGrepLimit
	}
	if c.files == nil {
		return GrepResult{}, errors.New("workspace: file browser is not configured")
	}
	return c.files.Grep(ctx, root, input)
}

func previewLines(read FileReadResult) []FileLine {
	if read.Content == "" && read.TotalLines == 0 {
		return []FileLine{}
	}
	parts := strings.Split(read.Content, "\n")
	lines := make([]FileLine, 0, len(parts))
	for index, text := range parts {
		lines = append(lines, FileLine{Number: read.StartLine + index + 1, Text: text})
	}
	return lines
}

func pageFileEntries(entries []FileEntry, cursor string, limit int) ([]FileEntry, string, error) {
	if limit < 0 {
		return nil, "", fmt.Errorf("%w: limit must not be negative", ErrPageLimit)
	}
	slices.SortFunc(entries, func(a, b FileEntry) int { return cmp.Compare(a.orderKey(), b.orderKey()) })
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(decoded) == 0 {
			return nil, "", ErrPageCursor
		}
		anchor := string(decoded)
		start := len(entries)
		for index, entry := range entries {
			if candidate := entry.orderKey(); candidate >= anchor {
				start = index
				if candidate == anchor {
					start++
				}
				break
			}
		}
		entries = entries[start:]
	}
	if limit <= 0 || limit > defaultFileListPageLimit {
		limit = defaultFileListPageLimit
	}
	end := min(limit, len(entries))
	page := slices.Clone(entries[:end])
	if end == len(entries) {
		return page, "", nil
	}
	return page, base64.RawURLEncoding.EncodeToString([]byte(entries[end-1].orderKey())), nil
}

// FileStatus is the application vocabulary for a working-tree change. It is
// intentionally independent of both the Git adapter's type and the wire enum.
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileChange is one working-tree change.
type FileChange struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
}

// DiffRowType is the application vocabulary for a parsed unified-diff row.
type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

// DiffRow is one structured diff row.
type DiffRow struct {
	Type      DiffRowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}

// FileDiff is one file's structured diff.
type FileDiff struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
	Rows         []DiffRow
}

// GitReader is the application-owned port for working-tree status and diff
// reads. Its error contract uses this package's VCS sentinels.
type GitReader interface {
	ListFileChanges(ctx context.Context, root string) ([]FileChange, error)
	StructuredDiff(ctx context.Context, root, path string, base bool) ([]FileDiff, error)
	RawDiff(ctx context.Context, root, path string, base bool) (string, error)
}

// DiffInput selects a working-tree or merge-base diff, optionally as raw text.
type DiffInput struct {
	Cwd   string
	Path  string
	Base  bool
	Raw   bool
	Limit int
}

// Diff is a structured or raw workspace diff.
type Diff struct {
	Patch     string
	Files     []FileDiff
	Truncated bool
}

// ListFileChanges reads the root's VCS status.
func (c *VCS) ListFileChanges(ctx context.Context, cwd string) ([]FileChange, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.git == nil {
		return nil, ErrVCSUnavailable
	}
	return c.git.ListFileChanges(ctx, root)
}

// Diff reads a workspace VCS diff, keeping path confinement and file-boundary
// truncation in the application use case rather than the delivery projection.
func (c *VCS) Diff(ctx context.Context, input DiffInput) (Diff, error) {
	root, err := c.context.root(input.Cwd)
	if err != nil {
		return Diff{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = c.context.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return Diff{}, err
		}
	}
	if c.git == nil {
		return Diff{}, ErrVCSUnavailable
	}
	if input.Raw {
		patch, err := c.git.RawDiff(ctx, root, path, input.Base)
		return Diff{Patch: patch}, err
	}
	files, err := c.git.StructuredDiff(ctx, root, path, input.Base)
	if err != nil {
		return Diff{}, err
	}
	files, truncated := limitDiffRows(files, input.Limit)
	return Diff{Files: files, Truncated: truncated}, nil
}

func limitDiffRows(files []FileDiff, limit int) ([]FileDiff, bool) {
	if limit <= 0 {
		return files, false
	}
	rows := 0
	for index, file := range files {
		if index > 0 && rows+len(file.Rows) > limit {
			return files[:index], true
		}
		rows += len(file.Rows)
	}
	return files, false
}

// Project is a distinct workspace identity derived from user-facing sessions.
type Project struct {
	Name         string
	Cwd          string
	ProjectRoot  string
	CwdMissing   bool
	SessionCount int
	LastActiveAt time.Time
}

// ProjectCatalog supplies the user-facing sessions and their current workspace
// identities. The session coordinator is the production implementation.
type ProjectCatalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (session.WorkspaceIdentity, error)
}

// ListProjects returns each non-empty session cwd once, newest-active first.
func (c *Discovery) ListProjects(ctx context.Context) ([]Project, error) {
	if c.projects == nil {
		return nil, errors.New("workspace: project catalog is not configured")
	}
	sessions, err := c.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	projects := projectsFromSessions(sessions)
	for index := range projects {
		identity, err := c.projects.InspectWorkspace(projects[index].Cwd)
		if err != nil {
			return nil, err
		}
		projects[index].Cwd = identity.Cwd
		projects[index].ProjectRoot = identity.ProjectRoot
		projects[index].CwdMissing = identity.Missing
	}
	return projects, nil
}

func projectsFromSessions(sessions []session.Session) []Project {
	byCwd := map[string]*Project{}
	for _, session := range sessions {
		if session.Cwd == "" {
			continue
		}
		project := byCwd[session.Cwd]
		if project == nil {
			project = &Project{Cwd: session.Cwd, Name: filepath.Base(session.Cwd)}
			byCwd[session.Cwd] = project
		}
		project.SessionCount++
		if project.LastActiveAt.IsZero() || session.UpdatedAt.After(project.LastActiveAt) {
			project.LastActiveAt = session.UpdatedAt
		}
	}
	projects := make([]Project, 0, len(byCwd))
	for _, project := range byCwd {
		projects = append(projects, *project)
	}
	slices.SortFunc(projects, func(a, b Project) int { return b.LastActiveAt.Compare(a.LastActiveAt) })
	return projects
}

// AgentDocScope identifies where an instruction document participates in the
// cascade, without leaking a raw delivery enum through the application layer.
type AgentDocScope string

const (
	AgentDocScopeHome        AgentDocScope = "home"
	AgentDocScopeCwd         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
)

// AgentDoc is one discovered instruction document with its cascade scope.
type AgentDoc struct {
	Path  string
	Scope AgentDocScope
}

// AgentDocFinder discovers the workspace instruction-document cascade.
type AgentDocFinder interface {
	DiscoverAgentDocs(ctx context.Context, cwd, home string) ([]AgentDocFile, error)
}

// ListAgentDocs returns the instruction-document cascade for one workspace.
func (c *Discovery) ListAgentDocs(ctx context.Context, cwd string) ([]AgentDoc, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.agentDocs == nil {
		return nil, errors.New("workspace: agent document finder is not configured")
	}
	files, err := c.agentDocs.DiscoverAgentDocs(ctx, root, c.context.home)
	if err != nil {
		return nil, err
	}
	docs := make([]AgentDoc, 0, len(files))
	for _, file := range files {
		docs = append(docs, AgentDoc{Path: file.Path, Scope: agentDocScope(file.Path, root, c.context.home)})
	}
	return docs, nil
}

func agentDocScope(path, cwd, home string) AgentDocScope {
	dir := filepath.Dir(path)
	switch {
	case home != "" && dir == home:
		return AgentDocScopeHome
	case cwd != "" && (dir == cwd || strings.HasPrefix(path, cwd+string(filepath.Separator))):
		return AgentDocScopeCwd
	default:
		return AgentDocScopeProjectRoot
	}
}
