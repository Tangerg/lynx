package fileobservation

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

// TreeTarget identifies one bounded directory tree and the exact filename
// whose contents define its observable projection. Other files and directory
// metadata are deliberately ignored.
type TreeTarget struct {
	Key      string
	Path     string
	Boundary string
	FileName string
}

// WatchTrees observes a dynamic set of exact files below each target. It
// watches every current directory and the nearest existing ancestor of a
// missing tree, so additions, replacements, removals, and in-place writes all
// converge through one content-derived baseline.
func WatchTrees(targets []TreeTarget, notify func([]string)) (Observation, error) {
	canonical, err := canonicalTreeTargets(targets)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nopWatch{}, nil
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("observe trees: create watcher: %w", err)
	}
	w := &treeWatch{
		fsw: fsw, targets: canonical, notify: notify,
		baselines: make([]treeSnapshot, len(canonical)),
		watched:   make(map[string]struct{}),
		done:      make(chan struct{}),
		exited:    make(chan struct{}),
	}
	if err := w.reconcile(true, acceptance{}); err != nil {
		return nil, closeFailed(fsw, err)
	}
	go w.run()
	return w, nil
}

type treeTarget struct {
	key              string
	path             string
	physicalBoundary string
	fileName         string
}

func canonicalTreeTargets(targets []TreeTarget) ([]treeTarget, error) {
	out := make([]treeTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for index, candidate := range targets {
		if candidate.Key == "" {
			return nil, fmt.Errorf("observe trees: target %d key is required", index)
		}
		if candidate.Path == "" || !filepath.IsAbs(candidate.Path) {
			return nil, fmt.Errorf("observe trees: target %d path must be absolute", index)
		}
		if candidate.FileName == "" || filepath.Base(candidate.FileName) != candidate.FileName {
			return nil, fmt.Errorf("observe trees: target %d filename must be one path element", index)
		}
		path := filepath.Clean(candidate.Path)
		identity := candidate.Key + "\x00" + path + "\x00" + candidate.FileName
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		var boundary string
		if candidate.Boundary != "" {
			if !filepath.IsAbs(candidate.Boundary) {
				return nil, fmt.Errorf("observe trees: target %d boundary must be absolute", index)
			}
			resolved, err := pathidentity.Resolve("", candidate.Boundary)
			if err != nil {
				return nil, fmt.Errorf("observe trees: resolve target %d boundary: %w", index, err)
			}
			boundary = resolved
		}
		out = append(out, treeTarget{
			key: candidate.Key, path: path, physicalBoundary: boundary, fileName: candidate.FileName,
		})
	}
	return out, nil
}

type treeEntry struct {
	fingerprint fingerprint
	physical    string
}

type treeSnapshot map[string]treeEntry

type treeWatch struct {
	fsw       *fsnotify.Watcher
	targets   []treeTarget
	notify    func([]string)
	baselines []treeSnapshot
	watched   map[string]struct{}
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
	stateMu   sync.Mutex
	closed    bool
}

func (t *treeWatch) run() {
	defer close(t.exited)
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	armed := false
	armAfter := func(delay time.Duration) {
		if armed && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
		armed = true
	}
	for {
		select {
		case <-t.done:
			return
		case _, ok := <-t.fsw.Events:
			if !ok {
				return
			}
			armAfter(debounce)
		case _, ok := <-t.fsw.Errors:
			if !ok {
				return
			}
			armAfter(debounce)
		case <-timer.C:
			armed = false
			if err := t.reconcile(false, acceptance{}); err != nil {
				armAfter(retryDelay)
			}
		}
	}
}

func (t *treeWatch) reconcile(initial bool, accepted acceptance) error {
	t.stateMu.Lock()
	if t.closed {
		t.stateMu.Unlock()
		return nil
	}
	directories := make(map[string]struct{})
	next := make([]treeSnapshot, len(t.targets))
	changedKeys := make([]string, 0, len(t.targets))
	accepting := len(accepted.keys) > 0 && len(accepted.identities) > 0
	for index, candidate := range t.targets {
		state, watched, err := scanTree(candidate)
		if err != nil {
			t.stateMu.Unlock()
			return err
		}
		for _, directory := range watched {
			directories[directory] = struct{}{}
		}
		switch {
		case initial:
			next[index] = state
		case accepting:
			next[index] = acceptTreeChanges(candidate, t.baselines[index], state, accepted)
		default:
			next[index] = state
			if !treeSnapshotsEqual(state, t.baselines[index]) && !slices.Contains(changedKeys, candidate.key) {
				changedKeys = append(changedKeys, candidate.key)
			}
		}
	}
	if err := t.replaceDirectories(directories); err != nil {
		t.stateMu.Unlock()
		return err
	}
	t.baselines = next
	t.stateMu.Unlock()
	if !accepting && len(changedKeys) > 0 && t.notify != nil {
		t.notify(changedKeys)
	}
	return nil
}

func scanTree(candidate treeTarget) (treeSnapshot, []string, error) {
	physical, err := pathidentity.Resolve("", candidate.path)
	if err != nil {
		return nil, nil, fmt.Errorf("observe trees: resolve %q: %w", candidate.path, err)
	}
	if candidate.physicalBoundary != "" {
		inside, containsErr := pathidentity.Contains(candidate.physicalBoundary, physical)
		if containsErr != nil {
			return nil, nil, fmt.Errorf("observe trees: confine %q: %w", candidate.path, containsErr)
		}
		if !inside {
			directory, err := nearestExistingDirectory(filepath.Dir(candidate.path))
			if err != nil {
				return nil, nil, err
			}
			return treeSnapshot{}, []string{directory}, nil
		}
	}
	info, err := os.Stat(physical)
	if errors.Is(err, os.ErrNotExist) {
		directory, directoryErr := nearestExistingDirectory(filepath.Dir(physical))
		if directoryErr != nil {
			return nil, nil, directoryErr
		}
		return treeSnapshot{}, []string{directory}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("observe trees: inspect %q: %w", physical, err)
	}
	if !info.IsDir() {
		directory, directoryErr := nearestExistingDirectory(filepath.Dir(physical))
		if directoryErr != nil {
			return nil, nil, directoryErr
		}
		return treeSnapshot{}, []string{directory}, nil
	}

	snapshot := make(treeSnapshot)
	directories := make([]string, 0)
	err = filepath.WalkDir(physical, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if entry.Name() != candidate.fileName {
			return nil
		}
		relative, relErr := filepath.Rel(physical, path)
		if relErr != nil {
			return relErr
		}
		logical := filepath.Join(candidate.path, relative)
		value, resolved, relErr := fingerprintTreeFile(logical, path, candidate.physicalBoundary)
		if relErr != nil {
			return relErr
		}
		snapshot[logical] = treeEntry{fingerprint: value, physical: resolved}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("observe trees: scan %q: %w", physical, err)
	}
	if physical != candidate.path {
		logicalParent, err := nearestExistingDirectory(filepath.Dir(candidate.path))
		if err != nil {
			return nil, nil, err
		}
		directories = append(directories, logicalParent)
	}
	return snapshot, directories, nil
}

func fingerprintTreeFile(logical, physical, boundary string) (fingerprint, string, error) {
	resolved, err := pathidentity.Resolve("", physical)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe trees: resolve file %q: %w", logical, err)
	}
	if boundary != "" {
		inside, containsErr := pathidentity.Contains(boundary, resolved)
		if containsErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe trees: confine file %q: %w", logical, containsErr)
		}
		if !inside {
			return fingerprint{}, resolved, nil
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe trees: inspect file %q: %w", logical, err)
	}
	if !info.Mode().IsRegular() {
		hash := sha256.New()
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00type=%d", logical, resolved, info.Mode().Type())
		return sum(hash), resolved, nil
	}
	file, err := os.Open(resolved)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe trees: open %q: %w", logical, err)
	}
	hash := sha256.New()
	_, _ = io.WriteString(hash, logical+"\x00"+resolved+"\x00")
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return fingerprint{}, "", fmt.Errorf("observe trees: read %q: %w", logical, errors.Join(copyErr, closeErr))
	}
	return sum(hash), resolved, nil
}

func acceptTreeChanges(candidate treeTarget, baseline, current treeSnapshot, accepted acceptance) treeSnapshot {
	next := make(treeSnapshot, len(baseline))
	for logical, entry := range baseline {
		next[logical] = entry
	}
	if !accepted.keys[candidate.key] {
		return next
	}
	for logical, entry := range baseline {
		if accepted.identities[logical] || accepted.identities[entry.physical] {
			delete(next, logical)
		}
	}
	for logical, entry := range current {
		if accepted.identities[logical] || accepted.identities[entry.physical] {
			next[logical] = entry
		}
	}
	return next
}

func treeSnapshotsEqual(left, right treeSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for logical, leftEntry := range left {
		rightEntry, ok := right[logical]
		if !ok || leftEntry != rightEntry {
			return false
		}
	}
	return true
}

func (t *treeWatch) Accept(keys, identities []string) error {
	accepted := acceptance{
		keys:       make(map[string]bool, len(keys)),
		identities: make(map[string]bool, len(identities)),
	}
	for _, key := range keys {
		if key != "" {
			accepted.keys[key] = true
		}
	}
	for _, identity := range identities {
		if filepath.IsAbs(identity) {
			accepted.identities[filepath.Clean(identity)] = true
		}
	}
	if len(accepted.keys) == 0 || len(accepted.identities) == 0 {
		return nil
	}
	return t.reconcile(false, accepted)
}

func (t *treeWatch) replaceDirectories(next map[string]struct{}) error {
	for directory := range next {
		if _, present := t.watched[directory]; present {
			continue
		}
		if err := t.fsw.Add(directory); err != nil {
			return fmt.Errorf("observe trees: watch directory %q: %w", directory, err)
		}
		t.watched[directory] = struct{}{}
	}
	for directory := range t.watched {
		if _, keep := next[directory]; keep {
			continue
		}
		if err := t.fsw.Remove(directory); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			return fmt.Errorf("observe trees: unwatch directory %q: %w", directory, err)
		}
		delete(t.watched, directory)
	}
	return nil
}

func (t *treeWatch) Close() error {
	t.closeOnce.Do(func() {
		t.stateMu.Lock()
		t.closed = true
		t.stateMu.Unlock()
		close(t.done)
		<-t.exited
		_ = t.fsw.Close()
	})
	return nil
}
