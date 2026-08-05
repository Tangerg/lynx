package workspace

// FileChangeNotice identifies workspace files whose current state should be
// read again. It carries scope only; file contents and VCS state remain
// authoritative in their read use cases.
type FileChangeNotice struct {
	CWD   string
	Paths []string
}
