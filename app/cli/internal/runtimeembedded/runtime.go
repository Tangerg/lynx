// Package runtimeembedded adapts the public in-process runtime binding to the
// CLI-owned agent ports. Protocol DTOs and embedded lifecycle details stop at
// this package boundary.
package runtimeembedded

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

const clientName = "lyra-cli"

// Config contains the process-owned paths and build identity needed to open one
// embedded runtime. Paths retain the semantics documented by embedded.Config.
type Config struct {
	DataDirectory        string
	DefaultWorkspacePath string
	UserHomePath         string
	ConfigDirectories    []string
	ClientVersion        string
}

// Runtime is the anti-corruption adapter between protocol DTOs and CLI domain
// projections. It intentionally exposes no protocol or embedded types.
type Runtime struct {
	binding  *embedded.Runtime
	runs     runBinding
	meta     protocol.RequestMeta
	readFile func(string) ([]byte, error)
}

var _ agent.Runtime = (*Runtime)(nil)

// Open starts and validates one embedded runtime. A runtime whose discovery
// contract cannot support the CLI is closed before Open returns the error.
func Open(ctx context.Context, cfg Config) (*Runtime, error) {
	binding, err := embedded.Open(ctx, embedded.Config{
		DataDirectory:        cfg.DataDirectory,
		DefaultWorkspacePath: cfg.DefaultWorkspacePath,
		UserHomePath:         cfg.UserHomePath,
		ConfigDirectories:    slices.Clone(cfg.ConfigDirectories),
	})
	if err != nil {
		return nil, classifyError(err)
	}

	runtime := &Runtime{
		binding:  binding,
		runs:     binding,
		meta:     requestMeta(cfg.ClientVersion),
		readFile: readFile,
	}
	discovery, err := binding.Discover(ctx, runtime.callOptions())
	if err == nil {
		err = validateDiscovery(discovery)
	}
	if err != nil {
		return nil, errors.Join(classifyError(err), binding.Close())
	}
	return runtime, nil
}

func requestMeta(version string) protocol.RequestMeta {
	if version == "" {
		version = "dev"
	}
	return protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo:      &protocol.ClientInfo{Name: clientName, Version: version},
		ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{
				protocol.InterruptApproval,
				protocol.InterruptQuestion,
			},
			ExcludedEphemeralEvents: []protocol.SuppressibleRunEventType{
				protocol.SuppressibleRunSegmentProgress,
			},
		},
	}
}

func (r *Runtime) callOptions() embedded.CallOptions {
	return embedded.CallOptions{RequestMeta: cloneRequestMeta(r.meta)}
}

func (r *Runtime) commandOptions() (embedded.CommandOptions, error) {
	key, err := newIdempotencyKey()
	if err != nil {
		return embedded.CommandOptions{}, err
	}
	return embedded.CommandOptions{RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: key}, nil
}

func (r *Runtime) runCommandOptions() (embedded.RunCommandOptions, error) {
	key, err := newIdempotencyKey()
	if err != nil {
		return embedded.RunCommandOptions{}, err
	}
	return embedded.RunCommandOptions{RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: key}, nil
}

func (r *Runtime) subscriptionOptions(afterEventID string) embedded.RunSubscriptionOptions {
	return embedded.RunSubscriptionOptions{
		RequestMeta:  cloneRequestMeta(r.meta),
		AfterEventID: afterEventID,
	}
}

func cloneRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	cloned := meta
	if meta.ClientInfo != nil {
		cloned.ClientInfo = new(*meta.ClientInfo)
	}
	if meta.ClientCapabilities != nil {
		capabilities := *meta.ClientCapabilities
		capabilities.Features = make(map[string]protocol.FeaturePreference, len(meta.ClientCapabilities.Features))
		maps.Copy(capabilities.Features, meta.ClientCapabilities.Features)
		capabilities.InterruptTypes = slices.Clone(meta.ClientCapabilities.InterruptTypes)
		capabilities.ExcludedEphemeralEvents = slices.Clone(meta.ClientCapabilities.ExcludedEphemeralEvents)
		cloned.ClientCapabilities = &capabilities
	}
	return cloned
}

func newIdempotencyKey() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate runtime idempotency key: %w", err)
	}
	return "cli_" + hex.EncodeToString(entropy[:]), nil
}

func continueCursor(operation, current, next string) (string, error) {
	if next != "" && next == current {
		return "", fmt.Errorf("%s: runtime returned a non-advancing cursor", operation)
	}
	return next, nil
}

func validateDiscovery(discovery *protocol.DiscoverResponse) error {
	if discovery == nil {
		return fmt.Errorf("%w: discovery response is nil", agent.ErrIncompatibleRuntime)
	}
	if discovery.Protocol.Current != protocol.ProtocolVersion ||
		discovery.Protocol.MinSupported != protocol.ProtocolVersion {
		return fmt.Errorf(
			"%w: runtime serves %s..%s, CLI requires %s",
			agent.ErrIncompatibleRuntime,
			discovery.Protocol.MinSupported,
			discovery.Protocol.Current,
			protocol.ProtocolVersion,
		)
	}
	if discovery.Capabilities.Limits.RunReplay.Scope != protocol.ReplayScopeRuntimeInstanceRootSegment {
		return fmt.Errorf("%w: unsupported run replay scope %q", agent.ErrIncompatibleRuntime, discovery.Capabilities.Limits.RunReplay.Scope)
	}
	for _, method := range []string{"runs.start", "runs.resume", "runs.subscribe"} {
		if !slices.Contains(discovery.Capabilities.StreamingMethods, method) {
			return fmt.Errorf("%w: runtime does not stream %s", agent.ErrIncompatibleRuntime, method)
		}
	}
	for _, eventType := range []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamStateSnapshot,
	} {
		if !slices.Contains(discovery.Capabilities.RunEvents, eventType) {
			return fmt.Errorf("%w: runtime does not advertise %s", agent.ErrIncompatibleRuntime, eventType)
		}
	}
	if !slices.ContainsFunc(discovery.Capabilities.StateSnapshots, func(capability protocol.StateSnapshotCapability) bool {
		return capability.Key == protocol.StatePlan &&
			capability.RecoveryMethod == "plan.get" &&
			capability.Scope == protocol.StateScopeSession &&
			capability.Writer == protocol.StateWriterRootRun
	}) {
		return fmt.Errorf("%w: runtime does not expose the required plan projection", agent.ErrIncompatibleRuntime)
	}
	return nil
}

// Close completes the embedded runtime teardown. Call it again when it returns
// an error; embedded.Close resumes incomplete teardown.
func (r *Runtime) Close() error {
	if r == nil || r.binding == nil {
		return nil
	}
	return classifyError(r.binding.Close())
}

// Owner lazily opens exactly one Runtime for a process and owns its shutdown.
// A failed Open may be retried; after Close begins, no new Runtime is opened.
type Owner struct {
	mu      sync.Mutex
	config  Config
	runtime *Runtime
	closing bool
}

func NewOwner(config Config) *Owner {
	config.ConfigDirectories = slices.Clone(config.ConfigDirectories)
	return &Owner{config: config}
}

func (o *Owner) Runtime(ctx context.Context) (agent.Runtime, error) {
	if o == nil {
		return nil, errors.New("embedded runtime owner is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return nil, agent.ErrDisconnected
	}
	if o.runtime != nil {
		return o.runtime, nil
	}
	opened, err := Open(ctx, o.config)
	if err != nil {
		return nil, err
	}
	o.runtime = opened
	return opened, nil
}

func (o *Owner) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closing = true
	runtime := o.runtime
	if runtime == nil {
		return nil
	}
	return runtime.Close()
}
