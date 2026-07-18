package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	snapshotPrefix       = "sha256:"
	maxSnapshotBytes     = 512 << 20
	maxSnapshotFileBytes = 128 << 20
	maxSnapshotEntries   = 100_000
)

// SnapshotID is the content digest of a deterministic workspace tar archive.
type SnapshotID string

// String returns the durable content-addressed reference.
func (id SnapshotID) String() string { return string(id) }

// Validate rejects malformed durable references before storage lookup.
func (id SnapshotID) Validate() error {
	raw, ok := strings.CutPrefix(string(id), snapshotPrefix)
	if !ok || len(raw) != sha256.Size*2 {
		return fmt.Errorf("sandbox: invalid snapshot id %q", id)
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return fmt.Errorf("sandbox: invalid snapshot id %q: %w", id, err)
	}
	return nil
}

func identifySnapshot(archive []byte) SnapshotID {
	digest := sha256.Sum256(archive)
	return SnapshotID(snapshotPrefix + hex.EncodeToString(digest[:]))
}

func archiveTree(ctx context.Context, root string) ([]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", root)
	}

	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	entries := 0
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entries++
		if entries > maxSnapshotEntries {
			return fmt.Errorf("workspace has more than %d entries", maxSnapshotEntries)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name:   filepath.ToSlash(rel),
			Mode:   int64(info.Mode().Perm()),
			Format: tar.FormatPAX,
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			header.Typeflag = tar.TypeDir
		case mode.IsRegular():
			if info.Size() > maxSnapshotFileBytes {
				return fmt.Errorf("file %q is %d bytes; limit is %d", rel, info.Size(), maxSnapshotFileBytes)
			}
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
		case mode&os.ModeSymlink != 0:
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			if err := validateSymlinkTarget(filepath.ToSlash(rel), filepath.ToSlash(target)); err != nil {
				return err
			}
			header.Typeflag = tar.TypeSymlink
			header.Linkname = filepath.ToSlash(target)
		default:
			return fmt.Errorf("unsupported file type %s at %q", mode.Type(), rel)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !mode.IsRegular() {
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if archive.Len() > maxSnapshotBytes {
			return fmt.Errorf("workspace archive exceeds %d bytes", maxSnapshotBytes)
		}
		return nil
	})
	if err != nil {
		_ = tw.Close()
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if archive.Len() > maxSnapshotBytes {
		return nil, fmt.Errorf("workspace archive exceeds %d bytes", maxSnapshotBytes)
	}
	return archive.Bytes(), nil
}

type directoryMode struct {
	name string
	mode fs.FileMode
}

func extractArchive(ctx context.Context, destination string, archive []byte) error {
	if len(archive) > maxSnapshotBytes {
		return fmt.Errorf("archive is %d bytes; limit is %d", len(archive), maxSnapshotBytes)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()

	tr := tar.NewReader(bytes.NewReader(archive))
	seen := make(map[string]struct{})
	var directories []directoryMode
	entries := 0
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		entries++
		if entries > maxSnapshotEntries {
			return fmt.Errorf("archive has more than %d entries", maxSnapshotEntries)
		}
		name, err := cleanArchiveName(header.Name)
		if err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("archive contains duplicate path %q", name)
		}
		seen[name] = struct{}{}
		mode := fs.FileMode(header.Mode) & fs.ModePerm
		parent := path.Dir(name)
		if parent != "." {
			if err := root.MkdirAll(filepath.FromSlash(parent), 0o700); err != nil {
				return fmt.Errorf("create parent for %q: %w", name, err)
			}
		}
		rootName := filepath.FromSlash(name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(rootName, 0o700); err != nil {
				return fmt.Errorf("create directory %q: %w", name, err)
			}
			directories = append(directories, directoryMode{name: rootName, mode: mode})
		case tar.TypeReg:
			if header.Size < 0 || header.Size > maxSnapshotFileBytes {
				return fmt.Errorf("archive file %q has invalid size %d", name, header.Size)
			}
			total += header.Size
			if total > maxSnapshotBytes {
				return fmt.Errorf("archive content exceeds %d bytes", maxSnapshotBytes)
			}
			file, err := root.OpenFile(rootName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create file %q: %w", name, err)
			}
			_, copyErr := io.CopyN(file, tr, header.Size)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return fmt.Errorf("write file %q: %w", name, errors.Join(copyErr, closeErr))
			}
		case tar.TypeSymlink:
			if err := validateSymlinkTarget(name, header.Linkname); err != nil {
				return err
			}
			if err := root.Symlink(filepath.FromSlash(header.Linkname), rootName); err != nil {
				return fmt.Errorf("create symlink %q: %w", name, err)
			}
		default:
			return fmt.Errorf("archive path %q uses unsupported type %d", name, header.Typeflag)
		}
	}
	// Restore restrictive directory modes after all children exist. Deepest
	// directories go first so a read-only parent never blocks a child chmod.
	slices.Reverse(directories)
	for _, directory := range directories {
		if err := root.Chmod(directory.name, directory.mode); err != nil {
			return fmt.Errorf("chmod directory %q: %w", directory.name, err)
		}
	}
	return nil
}

func cleanArchiveName(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') || path.IsAbs(name) {
		return "", fmt.Errorf("archive contains invalid path %q", name)
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != name {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	return cleaned, nil
}

func validateSymlinkTarget(name, target string) error {
	if target == "" || strings.ContainsRune(target, '\x00') || path.IsAbs(target) {
		return fmt.Errorf("archive symlink %q has unsafe target %q", name, target)
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("archive symlink %q escapes the workspace via %q", name, target)
	}
	return nil
}
