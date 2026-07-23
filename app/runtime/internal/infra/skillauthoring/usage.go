package skillauthoring

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// usageFile is the store-root sidecar holding per-skill usage. Its dot-prefixed
// name is not a valid skill name, so the read-only loader and List skip it. One
// central file (not per-skill dotfiles) keeps writes serialized under the store
// mutex and reads cheap for the curator sweep.
const usageFile = ".usage.json"

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
func (r usageRecord) lastActivity() int64 {
	if r.LastUsed > r.FirstSeen {
		return r.LastUsed
	}
	return r.FirstSeen
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
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()

	usage, err := readUsage(root)
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
	return writeUsage(root, usage)
}

// SweepIdle archives agent-authored skills idle past archiveAfter, returning the
// names it archived. It is provenance-gated: only skills whose frontmatter
// created_by is [skills.CreatedByAgent] are ever auto-curated — a human-authored
// skill is left untouched. Archiving moves the skill to _archive (never deletes)
// and drops its usage record, so a later restore starts with a fresh grace floor
// rather than being re-archived on the next sweep. A skill with no record yet is
// seeded at now (persisted), giving it the full archiveAfter grace anchored from
// its first sweep before it can be judged idle. now is explicit so the policy
// stays testable.
func (s *Store) SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error) {
	if !s.Enabled() {
		return nil, nil
	}
	if err := contextError(ctx, "sweep skills"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()

	names, err := activeSkillNames(root)
	if err != nil {
		return nil, err
	}
	usage, err := readUsage(root)
	if err != nil {
		return nil, err
	}
	var archived []string
	for _, name := range names {
		content, found, err := readSkill(root, name)
		if err != nil {
			return archived, err
		}
		if !found {
			continue
		}
		front, _, err := skillspec.Parse(content)
		if err != nil {
			continue
		}
		if front.Metadata[metadataCreatedBy] != skills.CreatedByAgent {
			continue // provenance gate: only agent-authored skills auto-curate
		}
		record := usage[name]
		if record.FirstSeen == 0 {
			record.FirstSeen = now.Unix()
		}
		if now.Sub(time.Unix(record.lastActivity(), 0)) >= archiveAfter {
			if err := s.archiveActive(root, name); err != nil {
				return archived, err
			}
			delete(usage, name)
			archived = append(archived, name)
			continue
		}
		// Persist the (possibly just-seeded) FirstSeen so a never-used skill's
		// grace is anchored to its first sweep, not re-seeded to now every pass.
		usage[name] = record
	}
	if err := writeUsage(root, usage); err != nil {
		return archived, err
	}
	return archived, nil
}

// activeSkillNames lists the active skill directories directly under the store
// root — every directory that isn't the reserved _drafts/_archive area or a
// dotfile.
func activeSkillNames(root *os.Root) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: list active skills: %w", err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
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
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	usage, err := readUsage(root)
	if err != nil {
		return err
	}
	if _, ok := usage[name]; !ok {
		return nil
	}
	delete(usage, name)
	return writeUsage(root, usage)
}

func readUsage(root *os.Root) (map[string]usageRecord, error) {
	data, err := root.ReadFile(usageFile)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]usageRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skillauthoring: read usage: %w", err)
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
	return usage, nil
}

func writeUsage(root *os.Root, usage map[string]usageRecord) error {
	data, err := json.MarshalIndent(usage, "", "  ")
	if err != nil {
		return fmt.Errorf("skillauthoring: marshal usage: %w", err)
	}
	temporary := usageFile + ".tmp-" + rand.Text()
	if err := writeFile(root, temporary, data); err != nil {
		return err
	}
	if err := root.Rename(temporary, usageFile); err != nil {
		_ = root.Remove(temporary)
		return fmt.Errorf("skillauthoring: commit usage: %w", err)
	}
	return nil
}
