package workspace

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/gitprocess"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

// GitWatcher adapts platform filesystem notifications to the workspace
// application's Git-state observation port. It watches only .git signal
// directories (never a project tree), keeping the descriptor cost fixed even
// for large repositories.
type GitWatcher struct{}

var _ workspaceapp.GitStateWatcher = GitWatcher{}

const gitWatchDebounce = 200 * time.Millisecond

// Watch observes every distinct repository reached from roots. A
// non-repository root is intentionally inert: its diff view is unavailable as
// well, but the surrounding workspace subscription remains valid.
func (GitWatcher) Watch(roots []string, notify func()) (io.Closer, error) {
	repositories := watchedRepositories(roots)
	if len(repositories) == 0 {
		return nopWatch{}, nil
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &gitWatch{
		fsw: fsw, notify: notify, repositories: repositories,
		done: make(chan struct{}), exited: make(chan struct{}),
	}
	addedDirectories := make(map[string]struct{})
	for _, repository := range repositories {
		// The per-worktree git directory owns HEAD/index, while the common
		// directory owns packed refs and branch tips. They are identical for an
		// ordinary checkout and distinct for `git worktree` checkouts.
		for _, gitDir := range []string{repository.gitDir, repository.commonDir} {
			if err := addGitWatch(fsw, addedDirectories, gitDir, false); err != nil {
				return nil, closeFailedWatch(fsw, err)
			}
		}
		if err := addGitWatch(fsw, addedDirectories, filepath.Join(repository.commonDir, "refs", "heads"), true); err != nil {
			return nil, closeFailedWatch(fsw, err)
		}
	}
	go w.run()
	return w, nil
}

type watchedRepository struct {
	root        string
	gitDir      string
	commonDir   string
	fingerprint [sha256.Size]byte
	valid       bool
}

func watchedRepositories(roots []string) []watchedRepository {
	seen := make(map[string]struct{}, len(roots))
	repositories := make([]watchedRepository, 0, len(roots))
	for _, root := range roots {
		gitDir, commonDir, ok := gitDirectoriesOf(root)
		if !ok {
			continue
		}
		// Multiple WorkspaceRefs may point at different subtrees of the same
		// repository. Their fingerprints are intentionally separate because Git
		// pathspecs scope staged entries to each resource root.
		identity := filepath.Clean(root) + "\x00" + gitDir
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		fingerprint, valid := semanticGitFingerprint(root)
		repositories = append(repositories, watchedRepository{
			root: root, gitDir: gitDir, commonDir: commonDir,
			fingerprint: fingerprint, valid: valid,
		})
	}
	return repositories
}

func gitDirectoriesOf(root string) (gitDir, commonDir string, ok bool) {
	gitDirOutput, ok := gitObservation(root, "rev-parse", "--absolute-git-dir")
	if !ok {
		return "", "", false
	}
	gitDir = strings.TrimRight(string(gitDirOutput), "\r\n")
	commonDirOutput, ok := gitObservation(root, "rev-parse", "--git-common-dir")
	if !ok {
		return "", "", false
	}
	commonDir = strings.TrimRight(string(commonDirOutput), "\r\n")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	var err error
	gitDir, err = pathidentity.Resolve("", gitDir)
	if err != nil {
		return "", "", false
	}
	commonDir, err = pathidentity.Resolve("", commonDir)
	if err != nil {
		return "", "", false
	}
	for _, directory := range []string{gitDir, commonDir} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			return "", "", false
		}
	}
	return gitDir, commonDir, true
}

func addGitWatch(watcher *fsnotify.Watcher, added map[string]struct{}, directory string, optional bool) error {
	if _, exists := added[directory]; exists {
		return nil
	}
	if err := watcher.Add(directory); err != nil {
		if optional && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("watch git directory %q: %w", directory, err)
	}
	added[directory] = struct{}{}
	return nil
}

type gitWatch struct {
	fsw          *fsnotify.Watcher
	notify       func()
	repositories []watchedRepository
	done         chan struct{}
	exited       chan struct{}
	closeOnce    sync.Once
}

func (w *gitWatch) run() {
	defer close(w.exited)
	timer := time.NewTimer(gitWatchDebounce)
	defer timer.Stop()
	timer.Stop()
	armed := false
	for {
		select {
		case <-w.done:
			return
		case _, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !armed {
				timer.Reset(gitWatchDebounce)
				armed = true
			}
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// A transient overflow or removed ref directory does not invalidate the
			// subscription. The client will re-fetch on the next resync.
		case <-timer.C:
			armed = false
			if w.semanticStateChanged() && w.notify != nil {
				w.notify()
			}
		}
	}
}

// semanticStateChanged distinguishes Git state from Git's storage mechanics.
// Commands such as diff may refresh stat data by replacing .git/index even when
// HEAD and every staged entry are identical. Publishing that replacement as a
// change lets a diff refetch wake its own watcher forever. The watcher therefore
// compares the committed HEAD and stage entries that clients can actually read.
func (w *gitWatch) semanticStateChanged() bool {
	changed := false
	for index := range w.repositories {
		repository := &w.repositories[index]
		next, valid := semanticGitFingerprint(repository.root)
		if !valid || !repository.valid || next != repository.fingerprint {
			changed = true
		}
		repository.fingerprint = next
		repository.valid = valid
	}
	return changed
}

func semanticGitFingerprint(root string) ([sha256.Size]byte, bool) {
	head, headOK := gitObservation(root, "rev-parse", "--verify", "HEAD")
	if !headOK {
		// An unborn repository has no commit yet. Its symbolic ref still matters:
		// changing the branch name is a semantic move even before the first commit.
		head, headOK = gitObservation(root, "symbolic-ref", "--quiet", "HEAD")
		if !headOK {
			return [sha256.Size]byte{}, false
		}
	}
	index, ok := gitObservation(root, "ls-files", "--stage", "-z")
	if !ok {
		return [sha256.Size]byte{}, false
	}
	state := make([]byte, 0, len(head)+len(index)+2)
	state = append(state, head...)
	state = append(state, 0)
	state = append(state, index...)
	return sha256.Sum256(state), true
}

func gitObservation(root string, args ...string) ([]byte, bool) {
	full := append([]string{"--no-optional-locks", "-C", root}, args...)
	output, err := gitprocess.Command(full...).Output()
	return output, err == nil
}

// Close joins the callback goroutine before closing the underlying watcher, so
// a caller can safely close its output channel immediately afterwards.
func (w *gitWatch) Close() error {
	w.closeOnce.Do(func() {
		close(w.done)
		<-w.exited
		_ = w.fsw.Close()
	})
	return nil
}

func closeFailedWatch(watcher *fsnotify.Watcher, cause error) error {
	if err := watcher.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("close failed git watcher: %w", err))
	}
	return cause
}

type nopWatch struct{}

func (nopWatch) Close() error { return nil }
