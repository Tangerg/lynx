// Package checkpoint snapshots a session's working tree at run boundaries via
// a per-session SHADOW git repository, so a rollback or fork can restore files
// (not just chat history) to a chosen run.
//
// The shadow repo's GIT_DIR lives under the lyra home, with the session's cwd
// as its work tree — the user's own .git is never touched (git addresses the
// two independently, the classic dotfiles-repo pattern). Each run boundary is
// anchored by a lightweight tag named for the run id, so a restore is a reset
// to that tag. The only OS dependency is the git binary, which lyra already
// requires for workspace diffs — so this is platform-agnostic.
//
// To avoid re-hashing a project that git already has, a fresh shadow repo SEEDS
// itself from the real repo at cwd (see [Store.seedFrom]): it temporarily shares
// the real object store and copies its index. Once the first boundary commits,
// the reachable borrowed objects are packed into the shadow repo and the link is
// removed, so completed checkpoints remain self-contained.
package checkpoint

import (
	"errors"
)

// ErrUnavailable means there is no snapshot to restore for the requested run
// (no shadow repo, or no tag at that boundary). It maps to the wire
// checkpoint_unavailable.
var ErrUnavailable = errors.New("checkpoint: no snapshot for run")
