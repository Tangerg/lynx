package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maxWorkspaceCopyBytes     = 512 << 20
	maxWorkspaceCopyFileBytes = 128 << 20
	maxWorkspaceCopyEntries   = 100_000
	workspaceCopyBufferBytes  = 64 << 10
)

// copyTree materializes source directly into an empty destination. Both roots
// remain capability-confined for the whole copy; no archive-sized intermediate
// representation is created.
func copyTree(ctx context.Context, source, destination string) error {
	if ctx == nil {
		return errors.New("workspace copy context is required")
	}
	if !filepath.IsAbs(source) {
		return errors.New("workspace copy source must be absolute")
	}
	if !filepath.IsAbs(destination) {
		return errors.New("workspace copy destination must be absolute")
	}
	source = filepath.Clean(source)
	destination = filepath.Clean(destination)
	physicalSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("resolve workspace copy source: %w", err)
	}
	physicalDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return fmt.Errorf("resolve workspace copy destination: %w", err)
	}
	if containsPath(physicalSource, physicalDestination) {
		return errors.New("workspace copy destination must not be inside source")
	}

	sourceRoot, err := openDirectoryRoot(source, "source")
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	destinationRoot, err := openDirectoryRoot(destination, "destination")
	if err != nil {
		return err
	}
	defer destinationRoot.Close()

	copier := treeCopier{
		source:      sourceRoot,
		destination: destinationRoot,
		buffer:      make([]byte, workspaceCopyBufferBytes),
	}
	if err := fs.WalkDir(sourceRoot.FS(), ".", copier.copyEntry(ctx)); err != nil {
		return err
	}
	return copier.restoreDirectoryModes(ctx)
}

func openDirectoryRoot(name, role string) (*os.Root, error) {
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open workspace copy %s: %w", role, err)
	}
	info, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("stat workspace copy %s: %w", role, err)
	}
	if !info.IsDir() {
		_ = root.Close()
		return nil, fmt.Errorf("workspace copy %s %q is not a directory", role, name)
	}
	return root, nil
}

func containsPath(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type directoryMode struct {
	name string
	mode fs.FileMode
}

type treeCopier struct {
	source      *os.Root
	destination *os.Root
	buffer      []byte
	directories []directoryMode
	entries     int
	totalBytes  int64
}

func (t *treeCopier) copyEntry(ctx context.Context) fs.WalkDirFunc {
	return func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		t.entries++
		if t.entries > maxWorkspaceCopyEntries {
			return fmt.Errorf("workspace has more than %d entries", maxWorkspaceCopyEntries)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		portableName := filepath.ToSlash(name)
		localName := filepath.FromSlash(portableName)
		if parent := filepath.Dir(localName); parent != "." {
			if err := t.destination.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf("create parent for %q: %w", portableName, err)
			}
		}

		switch mode := info.Mode(); {
		case mode.IsDir():
			if err := t.destination.MkdirAll(localName, 0o700); err != nil {
				return fmt.Errorf("create directory %q: %w", portableName, err)
			}
			t.directories = append(t.directories, directoryMode{name: localName, mode: mode.Perm()})
			return nil
		case mode.IsRegular():
			return t.copyFile(ctx, portableName, localName, info)
		case mode&os.ModeSymlink != 0:
			return t.copySymlink(portableName, localName)
		default:
			return fmt.Errorf("unsupported file type %s at %q", mode.Type(), portableName)
		}
	}
}

func (t *treeCopier) copyFile(
	ctx context.Context,
	portableName string,
	localName string,
	info fs.FileInfo,
) error {
	size := info.Size()
	if size < 0 || size > maxWorkspaceCopyFileBytes {
		return fmt.Errorf("file %q is %d bytes; limit is %d", portableName, size, maxWorkspaceCopyFileBytes)
	}
	if size > maxWorkspaceCopyBytes-t.totalBytes {
		return fmt.Errorf("workspace content exceeds %d bytes", maxWorkspaceCopyBytes)
	}
	t.totalBytes += size

	source, err := t.source.Open(localName)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", portableName, err)
	}
	openedInfo, statErr := source.Stat()
	if statErr != nil || !openedInfo.Mode().IsRegular() || openedInfo.Size() != size || !os.SameFile(info, openedInfo) {
		closeErr := source.Close()
		if statErr != nil {
			return fmt.Errorf("restat source file %q: %w", portableName, errors.Join(statErr, closeErr))
		}
		return fmt.Errorf(
			"source file %q changed before copy: %w",
			portableName,
			errors.Join(errors.New("identity or size changed"), closeErr),
		)
	}
	destination, err := t.destination.OpenFile(
		localName,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		info.Mode().Perm(),
	)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", portableName, errors.Join(err, source.Close()))
	}

	reader := io.LimitReader(contextReader{ctx: ctx, reader: source}, size+1)
	written, copyErr := io.CopyBuffer(writeOnly{writer: destination}, reader, t.buffer)
	closeErr := errors.Join(destination.Close(), source.Close())
	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("copy file %q: %w", portableName, errors.Join(copyErr, closeErr))
	}
	if written != size {
		return fmt.Errorf("source file %q changed during copy: copied %d bytes, expected %d", portableName, written, size)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (c contextReader) Read(buffer []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.reader.Read(buffer)
}

type writeOnly struct{ writer io.Writer }

func (w writeOnly) Write(buffer []byte) (int, error) {
	return w.writer.Write(buffer)
}

func (t *treeCopier) copySymlink(portableName, localName string) error {
	target, err := t.source.Readlink(localName)
	if err != nil {
		return fmt.Errorf("read symlink %q: %w", portableName, err)
	}
	portableTarget := filepath.ToSlash(target)
	if err := validateSymlinkTarget(portableName, portableTarget); err != nil {
		return err
	}
	if err := t.destination.Symlink(filepath.FromSlash(portableTarget), localName); err != nil {
		return fmt.Errorf("create symlink %q: %w", portableName, err)
	}
	return nil
}

func (t *treeCopier) restoreDirectoryModes(ctx context.Context) error {
	slices.Reverse(t.directories)
	for _, directory := range t.directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.destination.Chmod(directory.name, directory.mode); err != nil {
			return fmt.Errorf("chmod directory %q: %w", directory.name, err)
		}
	}
	return nil
}

func validateSymlinkTarget(name, target string) error {
	platformTarget := filepath.FromSlash(target)
	if target == "" ||
		strings.ContainsRune(target, '\x00') ||
		strings.ContainsRune(target, '\\') ||
		isAbsolutePortablePath(target, platformTarget) {
		return fmt.Errorf("workspace symlink %q has unsafe target %q", name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("workspace symlink %q escapes via %q", name, target)
	}
	return nil
}

func isAbsolutePortablePath(portableName, platformName string) bool {
	if path.IsAbs(portableName) ||
		filepath.IsAbs(platformName) ||
		filepath.VolumeName(platformName) != "" {
		return true
	}
	return len(portableName) >= 2 &&
		((portableName[0] >= 'a' && portableName[0] <= 'z') ||
			(portableName[0] >= 'A' && portableName[0] <= 'Z')) &&
		portableName[1] == ':'
}
