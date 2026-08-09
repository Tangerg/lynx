package sandbox

import (
	"archive/tar"
	"bytes"
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
	maxWorkspaceArchiveBytes     = 512 << 20
	maxWorkspaceArchiveFileBytes = 128 << 20
	maxWorkspaceArchiveEntries   = 100_000
)

func archiveTree(ctx context.Context, root string) ([]byte, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("archive root must be absolute")
	}
	root = filepath.Clean(root)
	archiveRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open archive root: %w", err)
	}
	defer archiveRoot.Close()
	info, err := archiveRoot.Stat(".")
	if err != nil {
		return nil, fmt.Errorf("stat archive root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", root)
	}

	archiver := newTreeArchiver(archiveRoot)
	err = fs.WalkDir(archiveRoot.FS(), ".", archiver.writeEntry(ctx))
	if err != nil {
		_ = archiver.close()
		return nil, err
	}
	if err := archiver.close(); err != nil {
		return nil, err
	}
	if archiver.archive.Len() > maxWorkspaceArchiveBytes {
		return nil, fmt.Errorf("workspace archive exceeds %d bytes", maxWorkspaceArchiveBytes)
	}
	return archiver.archive.Bytes(), nil
}

type treeArchiver struct {
	root    *os.Root
	archive bytes.Buffer
	writer  *tar.Writer
	entries int
}

func newTreeArchiver(root *os.Root) *treeArchiver {
	archiver := &treeArchiver{root: root}
	archiver.writer = tar.NewWriter(&archiver.archive)
	return archiver
}

func (archiver *treeArchiver) close() error {
	return archiver.writer.Close()
}

func (archiver *treeArchiver) writeEntry(ctx context.Context) fs.WalkDirFunc {
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
		archiver.entries++
		if archiver.entries > maxWorkspaceArchiveEntries {
			return fmt.Errorf("workspace has more than %d entries", maxWorkspaceArchiveEntries)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := archiver.header(name, info)
		if err != nil {
			return err
		}
		if err := archiver.writer.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			return archiver.writeFile(name)
		}
		return nil
	}
}

func (archiver *treeArchiver) header(name string, info fs.FileInfo) (*tar.Header, error) {
	archiveName := filepath.ToSlash(name)
	header := &tar.Header{Name: archiveName, Mode: int64(info.Mode().Perm()), Format: tar.FormatPAX}
	switch mode := info.Mode(); {
	case mode.IsDir():
		header.Typeflag = tar.TypeDir
	case mode.IsRegular():
		if info.Size() > maxWorkspaceArchiveFileBytes {
			return nil, fmt.Errorf("file %q is %d bytes; limit is %d", name, info.Size(), maxWorkspaceArchiveFileBytes)
		}
		header.Typeflag = tar.TypeReg
		header.Size = info.Size()
	case mode&os.ModeSymlink != 0:
		target, err := archiver.root.Readlink(name)
		if err != nil {
			return nil, err
		}
		header.Linkname = filepath.ToSlash(target)
		if err := validateSymlinkTarget(archiveName, header.Linkname); err != nil {
			return nil, err
		}
		header.Typeflag = tar.TypeSymlink
	default:
		return nil, fmt.Errorf("unsupported file type %s at %q", mode.Type(), name)
	}
	return header, nil
}

func (archiver *treeArchiver) writeFile(name string) error {
	file, err := archiver.root.Open(name)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(archiver.writer, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if archiver.archive.Len() > maxWorkspaceArchiveBytes {
		return fmt.Errorf("workspace archive exceeds %d bytes", maxWorkspaceArchiveBytes)
	}
	return nil
}

type directoryMode struct {
	name string
	mode fs.FileMode
}

func extractArchive(ctx context.Context, destination string, archive []byte) error {
	if len(archive) > maxWorkspaceArchiveBytes {
		return fmt.Errorf("archive is %d bytes; limit is %d", len(archive), maxWorkspaceArchiveBytes)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()

	extractor := archiveExtractor{
		root:   root,
		reader: tar.NewReader(bytes.NewReader(archive)),
		seen:   make(map[string]struct{}),
	}
	if err := extractor.extract(ctx); err != nil {
		return err
	}
	return extractor.restoreDirectoryModes()
}

type archiveExtractor struct {
	root        *os.Root
	reader      *tar.Reader
	seen        map[string]struct{}
	directories []directoryMode
	entries     int
	totalBytes  int64
}

func (extractor *archiveExtractor) extract(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := extractor.reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		extractor.entries++
		if extractor.entries > maxWorkspaceArchiveEntries {
			return fmt.Errorf("archive has more than %d entries", maxWorkspaceArchiveEntries)
		}
		if err := extractor.extractHeader(header); err != nil {
			return err
		}
	}
}

func (extractor *archiveExtractor) extractHeader(header *tar.Header) error {
	name, mode, err := extractor.validateHeader(header)
	if err != nil {
		return err
	}
	parent := path.Dir(name)
	if parent != "." {
		if err := extractor.root.MkdirAll(filepath.FromSlash(parent), 0o700); err != nil {
			return fmt.Errorf("create parent for %q: %w", name, err)
		}
	}
	rootName := filepath.FromSlash(name)
	switch header.Typeflag {
	case tar.TypeDir:
		if err := extractor.root.MkdirAll(rootName, 0o700); err != nil {
			return fmt.Errorf("create directory %q: %w", name, err)
		}
		extractor.directories = append(extractor.directories, directoryMode{name: rootName, mode: mode})
	case tar.TypeReg:
		return extractor.extractFile(name, rootName, mode, header.Size)
	case tar.TypeSymlink:
		if err := validateSymlinkTarget(name, header.Linkname); err != nil {
			return err
		}
		if err := extractor.root.Symlink(filepath.FromSlash(header.Linkname), rootName); err != nil {
			return fmt.Errorf("create symlink %q: %w", name, err)
		}
	default:
		return fmt.Errorf("archive path %q uses unsupported type %d", name, header.Typeflag)
	}
	return nil
}

func (extractor *archiveExtractor) validateHeader(header *tar.Header) (string, fs.FileMode, error) {
	name, err := cleanArchiveName(header.Name)
	if err != nil {
		return "", 0, err
	}
	if _, duplicate := extractor.seen[name]; duplicate {
		return "", 0, fmt.Errorf("archive contains duplicate path %q", name)
	}
	extractor.seen[name] = struct{}{}
	if header.Mode < 0 || header.Mode&^int64(fs.ModePerm) != 0 {
		return "", 0, fmt.Errorf("archive path %q has invalid permission mode %#o", name, header.Mode)
	}
	return name, fs.FileMode(header.Mode), nil //nolint:gosec // the preceding mask check proves the value fits FileMode
}

func (extractor *archiveExtractor) extractFile(name, rootName string, mode fs.FileMode, size int64) error {
	if size < 0 || size > maxWorkspaceArchiveFileBytes {
		return fmt.Errorf("archive file %q has invalid size %d", name, size)
	}
	extractor.totalBytes += size
	if extractor.totalBytes > maxWorkspaceArchiveBytes {
		return fmt.Errorf("archive content exceeds %d bytes", maxWorkspaceArchiveBytes)
	}
	file, err := extractor.root.OpenFile(rootName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create file %q: %w", name, err)
	}
	_, copyErr := io.CopyN(file, extractor.reader, size)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("write file %q: %w", name, errors.Join(copyErr, closeErr))
	}
	return nil
}

func (extractor *archiveExtractor) restoreDirectoryModes() error {
	// Restore restrictive directory modes after all children exist. Deepest
	// directories go first so a read-only parent never blocks a child chmod.
	slices.Reverse(extractor.directories)
	for _, directory := range extractor.directories {
		if err := extractor.root.Chmod(directory.name, directory.mode); err != nil {
			return fmt.Errorf("chmod directory %q: %w", directory.name, err)
		}
	}
	return nil
}

func cleanArchiveName(name string) (string, error) {
	platformName := filepath.FromSlash(name)
	if name == "" ||
		strings.ContainsRune(name, '\x00') ||
		strings.ContainsRune(name, '\\') ||
		isArchiveAbsolutePath(name, platformName) {
		return "", fmt.Errorf("archive contains invalid path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != name {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	return cleaned, nil
}

func validateSymlinkTarget(name, target string) error {
	platformTarget := filepath.FromSlash(target)
	if target == "" ||
		strings.ContainsRune(target, '\x00') ||
		strings.ContainsRune(target, '\\') ||
		isArchiveAbsolutePath(target, platformTarget) {
		return fmt.Errorf("archive symlink %q has unsafe target %q", name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("archive symlink %q escapes the workspace via %q", name, target)
	}
	return nil
}

func isArchiveAbsolutePath(portableName, platformName string) bool {
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
