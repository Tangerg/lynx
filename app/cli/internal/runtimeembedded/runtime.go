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
	"github.com/Tangerg/lynx/app/cli/internal/backend"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

const clientName = "lyra-cli"

func supportedInterruptTypes() []protocol.InterruptType {
	return []protocol.InterruptType{
		protocol.InterruptApproval,
		protocol.InterruptQuestion,
	}
}

func recognizedRunEventTypes() []protocol.StreamEventType {
	return []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentProgress,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamStateSnapshot,
		protocol.StreamCustom,
	}
}

func requiredRunEventTypes() []protocol.StreamEventType {
	return []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamStateSnapshot,
	}
}

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
	binding          *embedded.Runtime
	modelCatalog     modelCatalogBinding
	approvals        approvalBinding
	sessionCatalog   sessionCatalogBinding
	snapshot         snapshotBinding
	runCatalog       runCatalogBinding
	runs             runBinding
	sessions         sessionBinding
	workspaces       workspaceBinding
	changes          changeBinding
	usage            usageBinding
	modelConfig      modelConfigBinding
	goals            goalBinding
	skills           skillBinding
	mcp              mcpBinding
	schedules        scheduleBinding
	agentMemory      agentMemoryBinding
	knowledge        knowledgeBinding
	diagnosticTools  diagnosticToolBinding
	codebase         codebaseBinding
	authoringContext authoringContextBinding
	hooks            hookBinding
	feedback         feedbackBinding
	servicePortsOnce sync.Once
	agentMemoryPort  *agentMemoryAdapter
	knowledgePort    *knowledgeAdapter
	diagnosticPort   *diagnosticToolAdapter
	codebasePort     *codebaseAdapter
	authoringPort    *authoringContextAdapter
	hookPort         *hookAdapter
	feedbackPort     *feedbackAdapter
	meta             protocol.RequestMeta
	loadAttachment   attachmentLoader
	profile          runtimeprofile.Profile
}

var _ agent.Runtime = (*Runtime)(nil)
var _ workspace.Service = (*Runtime)(nil)
var _ changefeed.Source = (*Runtime)(nil)

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
		binding:          binding,
		modelCatalog:     binding,
		approvals:        binding,
		sessionCatalog:   binding,
		snapshot:         binding,
		runCatalog:       binding,
		runs:             binding,
		sessions:         binding,
		workspaces:       binding,
		changes:          binding,
		usage:            binding,
		modelConfig:      binding,
		goals:            binding,
		skills:           binding,
		mcp:              binding,
		schedules:        binding,
		agentMemory:      binding,
		knowledge:        binding,
		diagnosticTools:  binding,
		codebase:         binding,
		authoringContext: binding,
		hooks:            binding,
		feedback:         binding,
		meta:             requestMeta(cfg.ClientVersion),
		loadAttachment:   loadAttachmentFile,
	}
	discovery, err := binding.Discover(ctx, runtime.callOptions())
	if err == nil {
		err = validateDiscovery(discovery)
	}
	if err != nil {
		return nil, errors.Join(classifyError(err), binding.Close())
	}
	runtime.profile, err = projectRuntimeProfile(discovery, runtime.meta.ClientCapabilities)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("%w: %w", agent.ErrIncompatibleRuntime, err), binding.Close())
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
			Features: map[string]protocol.FeaturePreference{
				protocol.FeatureSubagents: {Enabled: true},
			},
			InterruptTypes: supportedInterruptTypes(),
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

func (r *Runtime) changeSubscriptionOptions() embedded.SubscriptionOptions {
	return embedded.SubscriptionOptions{RequestMeta: cloneRequestMeta(r.meta)}
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
	for _, eventType := range discovery.Capabilities.RunEvents {
		if !slices.Contains(recognizedRunEventTypes(), eventType) {
			return fmt.Errorf("%w: runtime advertises unsupported run event %q", agent.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, eventType := range requiredRunEventTypes() {
		if !slices.Contains(discovery.Capabilities.RunEvents, eventType) {
			return fmt.Errorf("%w: runtime does not advertise %s", agent.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, topic := range discovery.Capabilities.RuntimeTopics {
		if !slices.Contains(changefeed.Topics(), changefeed.Topic(topic)) {
			return fmt.Errorf("%w: runtime advertises unsupported change topic %q", agent.ErrIncompatibleRuntime, topic)
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

func (o *Owner) Runtime(ctx context.Context) (backend.Services, error) {
	if o == nil {
		return backend.Services{}, errors.New("embedded runtime owner is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return backend.Services{}, agent.ErrDisconnected
	}
	if o.runtime != nil {
		return o.runtime.services(), nil
	}
	opened, err := Open(ctx, o.config)
	if err != nil {
		return backend.Services{}, err
	}
	o.runtime = opened
	return opened.services(), nil
}

func (r *Runtime) services() backend.Services {
	r.servicePortsOnce.Do(func() {
		r.agentMemoryPort = &agentMemoryAdapter{runtime: r}
		r.knowledgePort = &knowledgeAdapter{runtime: r}
		r.diagnosticPort = &diagnosticToolAdapter{runtime: r}
		r.codebasePort = &codebaseAdapter{runtime: r}
		r.authoringPort = &authoringContextAdapter{runtime: r}
		r.hookPort = &hookAdapter{runtime: r}
		r.feedbackPort = &feedbackAdapter{runtime: r}
	})
	services := backend.Services{
		Agent: r, Workspaces: r, Changes: r, Transfers: r,
		Usage: r, ModelConfig: r, DiagnosticTools: r.diagnosticPort,
		AuthoringContext: r.authoringPort, Hooks: r.hookPort, Feedback: r.feedbackPort,
	}
	if r.profile.Available() {
		services.RuntimeProfile = new(r.profile.Clone())
	}
	if r.supportsFeature(protocol.FeatureGoals) {
		services.Goals = r
	}
	if r.supportsFeature(protocol.FeatureSkills) {
		services.Skills = r
	}
	if r.supportsFeature(protocol.FeatureMCP) {
		services.MCP = r
	}
	if r.supportsFeature(protocol.FeatureSchedules) {
		services.Schedules = r
	}
	if r.supportsFeature(protocol.FeatureAgentMemory) {
		services.AgentMemory = r.agentMemoryPort
	}
	if r.supportsFeature(protocol.FeatureKnowledge) {
		services.Knowledge = r.knowledgePort
	}
	if r.supportsFeature(protocol.FeatureCodebase) {
		services.Codebase = r.codebasePort
	}
	return services
}

func (r *Runtime) supportsFeature(name string) bool {
	return r.profile.Supports(name)
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
