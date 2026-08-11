// Package workspace owns the CLI's workspace inspection domain and the ports
// it asks a backend adapter to implement.
package workspace

import "context"

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
