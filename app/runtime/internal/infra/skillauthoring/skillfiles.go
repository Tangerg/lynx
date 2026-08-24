package skillauthoring

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

const skillFile = "SKILL.md"

func readSkill(root *os.Root, dir string) ([]byte, bool, error) {
	path := filepath.Join(dir, skillFile)
	content, found, err := readBoundedFile(root, path)
	if err != nil {
		return nil, false, fmt.Errorf("skillauthoring: read %q: %w", dir, err)
	}
	return content, found, nil
}

func readBoundedFile(root *os.Root, path string) ([]byte, bool, error) {
	file, err := root.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return nil, false, errors.Join(
			fmt.Errorf("inspect file: %w", statErr),
			file.Close(),
		)
	}
	if info.Size() > skills.MaxAuthoredSkillDocumentBytes {
		return nil, false, errors.Join(
			fmt.Errorf(
				"%w: %d bytes exceeds %d",
				skills.ErrDocumentTooLarge,
				info.Size(),
				skills.MaxAuthoredSkillDocumentBytes,
			),
			file.Close(),
		)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, skills.MaxAuthoredSkillDocumentBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, false, errors.Join(readErr, closeErr)
	}
	if len(content) > skills.MaxAuthoredSkillDocumentBytes {
		return nil, false, fmt.Errorf(
			"%w: exceeds %d bytes",
			skills.ErrDocumentTooLarge,
			skills.MaxAuthoredSkillDocumentBytes,
		)
	}
	return content, true, nil
}

// writeFile creates path (which must not exist) and writes+fsyncs content. It
// backs both proposal staging and the usage sidecar, so its messages name the
// operation neutrally; callers add the proposal/usage context.
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

func stageProposal(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	if err := root.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create proposal slot: %w", err)
	}
	temporary := filepath.Join(destination, ".stage-"+rand.Text())
	defer func() {
		if cleanupErr := root.Remove(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean proposal staging file: %w", cleanupErr))
		}
	}()
	if err := writeFile(root, temporary, content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish proposal"); err != nil {
		return err
	}
	if err := root.Rename(temporary, filepath.Join(destination, skillFile)); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			return nil
		}
		return fmt.Errorf("skillauthoring: publish proposal %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	return nil
}

func stageSkill(ctx context.Context, root *os.Root, destination string, content []byte) (err error) {
	temporary := ".skill-stage-" + rand.Text()
	if err := root.Mkdir(temporary, 0o755); err != nil {
		return fmt.Errorf("skillauthoring: create skill staging directory: %w", err)
	}
	defer func() {
		if cleanupErr := root.RemoveAll(temporary); cleanupErr != nil && !errors.Is(cleanupErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("skillauthoring: clean skill staging directory: %w", cleanupErr))
		}
	}()
	if err := writeFile(root, filepath.Join(temporary, skillFile), content); err != nil {
		return err
	}
	if err := contextError(ctx, "publish skill"); err != nil {
		return err
	}
	if err := root.Rename(temporary, destination); err != nil {
		existing, found, readErr := readSkill(root, destination)
		if readErr == nil && found && bytes.Equal(existing, content) {
			return nil
		}
		return fmt.Errorf("skillauthoring: publish skill %q: %w", filepath.Base(destination), errors.Join(err, readErr))
	}
	return nil
}

// removeProposal conditionally removes the current proposal only when its
// bytes still match ref.Revision. The caller owns the scoped library lease, so
// the compare and removal are one cross-process linearized decision.
func removeProposal(root *os.Root, ref skills.ProposalRef) (bool, error) {
	directory := filepath.Join(proposalsSubdir, ref.Name)
	content, found, err := readSkill(root, directory)
	if err != nil || !found {
		return false, err
	}
	if !ref.Matches(content) {
		return false, fmt.Errorf("%w: %q revision %q", skills.ErrProposalChanged, ref.Name, ref.Revision)
	}
	if err := root.RemoveAll(directory); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("skillauthoring: remove proposal %q: %w", ref.Name, err)
	}
	return true, nil
}
