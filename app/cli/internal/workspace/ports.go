// Package workspace owns the CLI's workspace inspection domain and the ports
// it asks a backend adapter to implement.
package workspace

import (
	"context"
	"errors"
)

// ErrVersionControlUnavailable means the workspace has no version-control
// projection. It is an expected workspace state, distinct from an empty change
// set and from a backend or transport failure.
var ErrVersionControlUnavailable = errors.New("version control unavailable")

// Service is the complete workspace capability assembled at the application
// boundary. Consumers should accept the narrower reader they actually use.
type Service interface {
	Catalog
	Inspector
}

type Catalog interface {
	Resolve(context.Context, ResolveRequest) (Workspace, error)
	List(context.Context) ([]Summary, error)
}

type Inspector interface {
	ChangeReader
	Diff(context.Context, DiffRequest) (Diff, error)
	Head(context.Context, HeadRequest) (FileHead, error)
	Search(context.Context, SearchRequest) (SearchResult, error)
	Files(context.Context, FilesRequest) (FileListing, error)
	Read(context.Context, ReadRequest) (FileContent, error)
}

type ChangeReader interface {
	Changes(context.Context, string) ([]Change, error)
}
