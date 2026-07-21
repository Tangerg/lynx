package skillauthoring

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// usageFile is the store-root sidecar holding per-skill usage. Its dot-prefixed
// name is not a valid skill name, so the read-only loader and List skip it. One
// central file (not per-skill dotfiles) keeps writes serialized under the store
// mutex and reads cheap for the curator sweep.
const usageFile = ".usage.json"

// usageState is a skill's lifecycle position, derived by the curator from
// inactivity and persisted for observability. Only Archived has a filesystem
// effect (the skill moves to _archive); Active/Stale are informational.
type usageState string

const usageActive usageState = "active"

// usageRecord tracks one skill's activity for the idle-lifecycle curator.
// FirstSeen anchors the grace floor for a never-used skill; LastUsed drives the
// stale/archive thresholds. Times are Unix seconds.
type usageRecord struct {
	FirstSeen int64      `json:"firstSeen"`
	LastUsed  int64      `json:"lastUsed,omitempty"`
	Uses      int        `json:"uses"`
	State     usageState `json:"state,omitempty"`
}

// RecordUse marks a skill loaded at now: it bumps the use count and last-used
// time (seeding FirstSeen on first sighting), so the curator can tell an
// actively-used skill from an idle one. Best-effort from the caller's side — a
// skill whose name has no active library entry simply leaves an inert record.
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
	record.Uses++
	record.State = usageActive
	usage[name] = record
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
