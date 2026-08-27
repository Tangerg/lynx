package skillauthoring

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/scope/app/runtime/internal/domain/skills"
)

// usageFile is the store-root sidecar holding per-skill usage. Its dot-prefixed
// name is not a valid skill name, so the read-only loader and List skip it. One
// central file (not per-skill dotfiles) keeps writes serialized under the store
// mutex and reads cheap for the curator sweep.
const (
	usageFile             = ".usage.json"
	maxUsageMetadataBytes = 64 << 10
)

// usageRecord tracks one skill's activity for the idle-lifecycle curator.
// FirstSeen anchors the grace floor for a never-used skill; LastUsed drives the
// archive threshold. Times are Unix seconds. (A stale/state field and a use
// count were dropped as write-only — nothing reads them yet; re-add with the
// lifecycle surface that would.)
type usageRecord struct {
	FirstSeen int64 `json:"firstSeen"`
	LastUsed  int64 `json:"lastUsed,omitempty"`
}

// lastActivity is the most recent signal of relevance — a load if the skill has
// been used, else when the store first saw it (so a brand-new, never-used skill
// gets the grace floor before it can be judged idle).
func (u usageRecord) lastActivity() int64 {
	if u.LastUsed > u.FirstSeen {
		return u.LastUsed
	}
	return u.FirstSeen
}

// RecordUse marks a skill loaded at now: it updates the last-used time (seeding
// FirstSeen on first sighting), so the curator can tell an actively-used skill
// from an idle one. Best-effort from the caller's side.
func (s *Store) RecordUse(ctx context.Context, name string, now time.Time) error {
	if !s.Enabled() {
		return nil
	}
	if err := contextError(ctx, "record skill use"); err != nil {
		return err
	}
	if !validName(name) {
		return fmt.Errorf("skillauthoring: invalid skill name %q", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "record skill use")
	if err != nil {
		return err
	}
	defer cleanup()

	usage, err := readUsage(ctx, root)
	if err != nil {
		return err
	}
	record := usage[name]
	ts := now.Unix()
	if record.FirstSeen == 0 {
		record.FirstSeen = ts
	}
	record.LastUsed = ts
	usage[name] = record
	return writeUsage(ctx, root, usage)
}

// SweepIdle archives Agent-authored skills idle past archiveAfter, returning the
// names it archived and every public file identity it changed. A valid proposal origin is the provenance gate; a
// human-authored Skill has none and is never auto-curated. Archiving moves the
// Skill to _archive (never deletes)
// and drops its usage record, so a later restore starts with a fresh grace floor
// rather than being re-archived on the next sweep. A skill with no record yet is
// seeded at now (persisted), giving it the full archiveAfter grace anchored from
// its first sweep before it can be judged idle. now is explicit so the policy
// stays testable.
func (s *Store) SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, []string, error) {
	if !s.Enabled() {
		return nil, nil, nil
	}
	if err := contextError(ctx, "sweep skills"); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "sweep skills")
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	names, err := activeSkillNames(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	usage, err := readUsage(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	var archived []string
	var identities []string
	for _, name := range names {
		content, found, err := readSkill(root, name)
		if err != nil {
			return archived, identities, err
		}
		if !found {
			continue
		}
		skill, err := skillspec.Parse(content)
		if err != nil {
			continue
		}
		if skills.ProposalOrigin(skill.Metadata[metadataOrigin]).Validate() != nil {
			continue // provenance gate: only agent-authored skills auto-curate
		}
		record := usage[name]
		if record.FirstSeen == 0 {
			record.FirstSeen = now.Unix()
		}
		if now.Sub(time.Unix(record.lastActivity(), 0)) >= archiveAfter {
			changed, err := s.archiveActive(root, name)
			identities = append(identities, changed...)
			if err != nil {
				return archived, distinctPaths(identities), err
			}
			delete(usage, name)
			archived = append(archived, name)
			continue
		}
		// Persist the (possibly just-seeded) FirstSeen so a never-used skill's
		// grace is anchored to its first sweep, not re-seeded to now every pass.
		usage[name] = record
	}
	if err := writeUsage(ctx, root, usage); err != nil {
		return archived, distinctPaths(identities), err
	}
	return archived, distinctPaths(identities), nil
}

// activeSkillNames lists the active skill directories directly under the store
// root — every directory that isn't the reserved _proposals/_archive area or a
// dotfile.
func activeSkillNames(ctx context.Context, root *os.Root) ([]string, error) {
	if err := contextError(ctx, "list active skills"); err != nil {
		return nil, err
	}
	active, _, err := managedSkillNames(root)
	return active, err
}

// dropUsage removes a skill's usage record if present. Used when a skill leaves
// the active set (archived), so a later restore is judged fresh rather than
// inheriting a stale last-used time.
func (s *Store) dropUsage(ctx context.Context, name string) error {
	if !s.Enabled() {
		return nil
	}
	if err := contextError(ctx, "drop skill usage"); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, cleanup, err := s.openLeasedRoot(ctx, "drop skill usage")
	if err != nil {
		return err
	}
	defer cleanup()
	usage, err := readUsage(ctx, root)
	if err != nil {
		return err
	}
	if _, ok := usage[name]; !ok {
		return nil
	}
	delete(usage, name)
	return writeUsage(ctx, root, usage)
}

func readUsage(ctx context.Context, root *os.Root) (map[string]usageRecord, error) {
	if err := contextError(ctx, "read skill usage"); err != nil {
		return nil, err
	}
	file, err := root.Open(usageFile)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]usageRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: open usage: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(fmt.Errorf("skillauthoring: inspect usage: %w", statErr), file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.New("skillauthoring: usage metadata is not a regular file"), file.Close())
	}
	if info.Size() > maxUsageMetadataBytes {
		return nil, errors.Join(
			fmt.Errorf("%w: %d bytes exceeds %d", skills.ErrUsageTooLarge, info.Size(), maxUsageMetadataBytes),
			file.Close(),
		)
	}
	data, readErr := io.ReadAll(io.LimitReader(
		skillUsageContextReader{ctx: ctx, reader: file},
		maxUsageMetadataBytes+1,
	))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("skillauthoring: read usage: %w", errors.Join(readErr, closeErr))
	}
	if len(data) > maxUsageMetadataBytes {
		return nil, fmt.Errorf("%w: exceeds %d bytes", skills.ErrUsageTooLarge, maxUsageMetadataBytes)
	}
	var usage map[string]usageRecord
	if err := json.Unmarshal(data, &usage); err != nil {
		// A corrupt usage file is non-critical metadata: start fresh rather than
		// wedging skill loads and curation on it.
		return map[string]usageRecord{}, nil
	}
	if usage == nil {
		usage = map[string]usageRecord{}
	}
	if len(usage) > skills.MaxSkillsPerSource {
		return nil, fmt.Errorf(
			"%w: usage metadata contains %d records; limit is %d",
			skills.ErrLibraryCapacity,
			len(usage),
			skills.MaxSkillsPerSource,
		)
	}
	for name := range usage {
		if !validName(name) {
			// Invalid records are non-critical corrupt metadata, like malformed
			// JSON: discard the sidecar rather than preserving unusable keys.
			return map[string]usageRecord{}, nil
		}
	}
	return usage, nil
}

func writeUsage(ctx context.Context, root *os.Root, usage map[string]usageRecord) error {
	if err := contextError(ctx, "write skill usage"); err != nil {
		return err
	}
	if len(usage) > skills.MaxSkillsPerSource {
		return fmt.Errorf(
			"%w: cannot write %d usage records; limit is %d",
			skills.ErrLibraryCapacity,
			len(usage),
			skills.MaxSkillsPerSource,
		)
	}
	data, err := json.MarshalIndent(usage, "", "  ")
	if err != nil {
		return fmt.Errorf("skillauthoring: marshal usage: %w", err)
	}
	if len(data) > maxUsageMetadataBytes {
		return fmt.Errorf("%w: encoded usage is %d bytes; limit is %d", skills.ErrUsageTooLarge, len(data), maxUsageMetadataBytes)
	}
	temporary := usageFile + ".tmp-" + rand.Text()
	if err := contextError(ctx, "write skill usage"); err != nil {
		return err
	}
	if err := writeFile(root, temporary, data); err != nil {
		return err
	}
	if err := contextError(ctx, "commit skill usage"); err != nil {
		_ = root.Remove(temporary)
		return err
	}
	if err := root.Rename(temporary, usageFile); err != nil {
		_ = root.Remove(temporary)
		return fmt.Errorf("skillauthoring: commit usage: %w", err)
	}
	return nil
}

type skillUsageContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (s skillUsageContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(s.ctx); cause != nil {
		return 0, cause
	}
	read, err := s.reader.Read(buffer)
	if cause := context.Cause(s.ctx); cause != nil {
		return read, cause
	}
	return read, err
}
