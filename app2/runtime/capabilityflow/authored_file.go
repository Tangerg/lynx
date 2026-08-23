package capabilityflow

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type authoredTarget struct {
	anchor   string
	path     string
	relative string
}

func readBoundedAuthoredFile(file *os.File, info fs.FileInfo) ([]byte, error) {
	if !info.Mode().IsRegular() {
		return nil, errors.New("authored resource is not a regular file")
	}
	if info.Size() > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("authored resource exceeds %d bytes", maxAuthoredDocumentBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAuthoredDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAuthoredDocumentBytes {
		return nil, fmt.Errorf("authored resource exceeds %d bytes", maxAuthoredDocumentBytes)
	}
	return data, nil
}

// confinedAuthoredTarget resolves an authored file through existing symlinks,
// proves the physical destination remains inside its declared root, and keeps
// the physical target identity so an in-root file symlink is not replaced.
func confinedAuthoredTarget(anchor string, relative string) (authoredTarget, error) {
	if !filepath.IsAbs(anchor) || filepath.IsAbs(relative) {
		return authoredTarget{}, protocol.ErrPathOutsideRoot
	}
	relative = filepath.Clean(relative)
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return authoredTarget{}, protocol.ErrPathOutsideRoot
	}
	physicalAnchor, err := filepath.EvalSymlinks(anchor)
	if err != nil {
		return authoredTarget{}, err
	}
	physicalAnchor, err = filepath.Abs(physicalAnchor)
	if err != nil {
		return authoredTarget{}, err
	}
	target := filepath.Join(anchor, relative)
	existing := target
	missing := make([]string, 0)
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return authoredTarget{}, err
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return authoredTarget{}, errors.New("capabilityflow: authored target has no existing anchor")
		}
		missing = append([]string{filepath.Base(existing)}, missing...)
		existing = parent
	}
	physicalExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return authoredTarget{}, err
	}
	physicalParts := append([]string{physicalExisting}, missing...)
	physicalPath := filepath.Join(physicalParts...)
	confined, err := filepath.Rel(physicalAnchor, physicalPath)
	if err != nil || confined == ".." || strings.HasPrefix(confined, ".."+string(filepath.Separator)) {
		return authoredTarget{}, protocol.ErrPathOutsideRoot
	}
	return authoredTarget{
		anchor: physicalAnchor, path: physicalPath, relative: confined,
	}, nil
}
