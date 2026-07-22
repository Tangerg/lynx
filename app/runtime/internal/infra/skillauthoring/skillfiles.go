package skillauthoring

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const skillFile = "SKILL.md"

func readSkill(root *os.Root, dir string) ([]byte, bool, error) {
	content, err := root.ReadFile(filepath.Join(dir, skillFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: read %q: %w", dir, err)
	}
	return content, true, nil
}

// writeFile creates path (which must not exist) and writes+fsyncs content. It
// backs both draft staging and the usage sidecar, so its messages name the
// operation neutrally; callers add the "draft"/"usage" context.
func writeFile(root *os.Root, path string, content []byte) (err error) {
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("skillauthoring: create %q: %w", path, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("skillauthoring: write %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("skillauthoring: sync %q: %w", path, err)
	}
	return nil
}

func stageDraft(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	temporary := filepath.Join(skills.DraftsSubdir, ".stage-"+rand.Text())
	if err := root.Mkdir(temporary, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create draft staging directory: %w", err)
	}
	defer func() {
		if cleanupErr := root.RemoveAll(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean draft staging directory: %w", cleanupErr))
		}
	}()
	if err := writeFile(root, filepath.Join(temporary, skillFile), content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish draft"); err != nil {
		return err
	}
	if err := root.Rename(temporary, destination); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			return nil
		}
		return fmt.Errorf("skillauthoring: publish draft %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	return nil
}
