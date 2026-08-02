package tools

// FileMutationReporter is an optional capability of tools that may change
// files. MutationPaths derives prospective targets from the same JSON
// arguments passed to Call. Hosts may use the result for locking and publish
// the paths only after a successful call.
type FileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}
