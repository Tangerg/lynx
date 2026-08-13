// Package runtimeownership maps application ownership identities to
// cross-process advisory leases rooted in one shared Runtime data directory.
package runtimeownership

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/ownershiprecovery"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/advisorylock"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

const ownershipDirectory = "ownership"

// Manager owns the stable lock-file layout shared by every Runtime process
// using the same data directory. Lock files carry no ownership state; the OS
// lock on their first byte is authoritative and is released on process death.
type Manager struct {
	sessions     string
	workingTrees string
	goalDrives   string
	recovery     string
}

// New prepares the private lock roots for one canonical data directory.
func New(dataDirectory string) (*Manager, error) {
	if dataDirectory == "" || !filepath.IsAbs(dataDirectory) {
		return nil, errors.New("session ownership: absolute data directory is required")
	}
	root := filepath.Join(filepath.Clean(dataDirectory), ownershipDirectory)
	manager := &Manager{
		sessions:     filepath.Join(root, "sessions"),
		workingTrees: filepath.Join(root, "working-trees"),
		goalDrives:   filepath.Join(root, "goal-drives"),
		recovery:     filepath.Join(root, "recovery"),
	}
	for _, directory := range []string{root, manager.sessions, manager.workingTrees, manager.goalDrives, manager.recovery} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("session ownership: create %q: %w", directory, err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, fmt.Errorf("session ownership: protect %q: %w", directory, err)
		}
	}
	return manager, nil
}

// TrySession acquires the exclusive writer lease for one Session.
func (m *Manager) TrySession(sessionID string) (sessionadmission.Lease, bool) {
	return tryLease(m.sessions, sessionID, false)
}

// TryWorkingTree acquires a shared Run lease or exclusive destructive-mutation
// lease for one physical working-tree identity.
func (m *Manager) TryWorkingTree(cwd string, shared bool) (sessionadmission.Lease, bool) {
	physical, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return nil, false
	}
	return tryLease(m.workingTrees, physical, shared)
}

// TryGoalDrive acquires the single autonomous driver lease for one Session.
func (m *Manager) TryGoalDrive(sessionID string) (goals.DriveLease, bool) {
	return tryLease(m.goalDrives, sessionID, false)
}

// TryRecoverySweep elects one Runtime to reconcile abandoned Runs before Goals.
func (m *Manager) TryRecoverySweep() (ownershiprecovery.Lease, bool) {
	lease, err := advisorylock.TryDirectory(m.recovery)
	if err != nil {
		return nil, false
	}
	return advisoryLease{lease: lease}, true
}

// AcquireRecoverySweep waits for the ordered startup recovery owner.
func (m *Manager) AcquireRecoverySweep(ctx context.Context) (ownershiprecovery.Lease, error) {
	lease, err := advisorylock.AcquireDirectory(ctx, m.recovery)
	if err != nil {
		return nil, err
	}
	return advisoryLease{lease: lease}, nil
}

type advisoryLease struct{ lease *advisorylock.Lease }

func (l advisoryLease) Release() { _ = l.lease.Release() }

type fileLease struct {
	file  *os.File
	lease *advisorylock.Lease
}

func (l *fileLease) Release() {
	if l == nil {
		return
	}
	if err := l.lease.Release(); err == nil {
		_ = l.file.Close()
	}
}

func tryLease(directory, identity string, shared bool) (*fileLease, bool) {
	if identity == "" {
		return nil, false
	}
	digest := sha256.Sum256([]byte(identity))
	path := filepath.Join(directory, hex.EncodeToString(digest[:])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	var lease *advisorylock.Lease
	if shared {
		lease, err = advisorylock.TrySharedFile(file)
	} else {
		lease, err = advisorylock.TryFile(file)
	}
	if err != nil {
		_ = file.Close()
		return nil, false
	}
	return &fileLease{file: file, lease: lease}, true
}

var (
	_ sessionadmission.Ownership  = (*Manager)(nil)
	_ goals.DriveOwnership        = (*Manager)(nil)
	_ ownershiprecovery.Ownership = (*Manager)(nil)
)
