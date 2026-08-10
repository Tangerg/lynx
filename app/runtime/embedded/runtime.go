package embedded

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/bootstrap"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

var (
	// ErrClosed is returned when an operation starts after Runtime shutdown has
	// begun. An already-returned stream ends through its iterator instead.
	ErrClosed = errors.New("embedded: runtime is closed")
	// ErrDataDirectoryInUse means another Runtime instance or process owns the
	// same canonical data directory.
	ErrDataDirectoryInUse = errors.New("embedded: data directory is already in use")
)

// Config identifies the host paths used by one embedded Runtime.
type Config struct {
	// DataDirectory holds Runtime durability and must be absolute. It is required
	// and exclusively owned until Close succeeds.
	DataDirectory string
	// DefaultWorkspacePath is used when a request omits its workspace. Empty uses
	// UserHomePath.
	DefaultWorkspacePath string
	// UserHomePath anchors home-scoped instructions and workspace discovery.
	// Empty takes one os.UserHomeDir snapshot during Open.
	UserHomePath string
	// ConfigDirectories are absolute search directories for config.yaml. Empty
	// searches only DataDirectory; the embedded binding never reads process cwd.
	ConfigDirectories []string
}

// Runtime is a complete in-process Lyra Runtime. It is safe for concurrent
// calls. Do not copy a Runtime; Open always returns a pointer.
type Runtime struct {
	mu       sync.RWMutex
	stopping bool
	instance *bootstrap.Instance
}

// Open constructs, recovers and starts one Runtime. The caller must Close it.
func Open(ctx context.Context, cfg Config) (*Runtime, error) {
	resolved, err := resolveConfig(cfg)
	if err != nil {
		return nil, err
	}
	instance, _, err := bootstrap.OpenInstance(ctx, bootstrap.InstanceConfig{
		UserHome:             resolved.UserHomePath,
		DefaultWorkspacePath: resolved.DefaultWorkspacePath,
		DataDirectory:        resolved.DataDirectory,
		ConfigDirectories:    resolved.ConfigDirectories,
		ServerInfo: protocol.ServerInfo{
			Name:    "runtime",
			Version: embeddedVersion(),
		},
	})
	if err != nil {
		if errors.Is(err, bootstrap.ErrDataDirectoryInUse) {
			return nil, fmt.Errorf("%w: %s", ErrDataDirectoryInUse, resolved.DataDirectory)
		}
		return nil, err
	}
	return &Runtime{instance: instance}, nil
}

func resolveConfig(cfg Config) (Config, error) {
	if cfg.DataDirectory == "" {
		return Config{}, errors.New("embedded: data directory is required")
	}
	if !filepath.IsAbs(cfg.DataDirectory) {
		return Config{}, errors.New("embedded: data directory must be absolute")
	}
	userHome := cfg.UserHomePath
	if userHome == "" {
		var err error
		userHome, err = os.UserHomeDir()
		if err != nil {
			return Config{}, fmt.Errorf("embedded: locate user home: %w", err)
		}
	}
	if userHome == "" || !filepath.IsAbs(userHome) {
		return Config{}, errors.New("embedded: user home must be a non-empty absolute path")
	}
	workspace := cfg.DefaultWorkspacePath
	if workspace == "" {
		workspace = userHome
	}
	if !filepath.IsAbs(workspace) {
		return Config{}, errors.New("embedded: default workspace path must be absolute")
	}
	directories := slices.Clone(cfg.ConfigDirectories)
	if len(directories) == 0 {
		directories = []string{cfg.DataDirectory}
	}
	for index, directory := range directories {
		if directory == "" || !filepath.IsAbs(directory) {
			return Config{}, errors.New("embedded: config directories must be non-empty absolute paths")
		}
		directories[index] = filepath.Clean(directory)
	}
	return Config{
		DataDirectory:        filepath.Clean(cfg.DataDirectory),
		DefaultWorkspacePath: filepath.Clean(workspace),
		UserHomePath:         filepath.Clean(userHome),
		ConfigDirectories:    directories,
	}, nil
}

func embeddedVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func (r *Runtime) endpoint() (*operation.Endpoint, error) {
	if r == nil {
		return nil, ErrClosed
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopping || r.instance == nil {
		return nil, ErrClosed
	}
	return r.instance.Endpoint(), nil
}

// Close stops new calls, ends subscriptions, joins Runtime-owned workers and
// closes resources before releasing the data-directory lease. If Close returns
// an error, call it again to resume the incomplete teardown.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.stopping = true
	instance := r.instance
	r.mu.Unlock()
	if instance == nil {
		return nil
	}
	return instance.Close()
}
