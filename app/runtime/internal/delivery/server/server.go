// Package server implements operation.Service by translating wire requests into
// application use cases and projecting their results back to protocol values.
package server

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// Config declares the application use cases, notification sources, and contract
// facts required to construct a Server.
type Config struct {
	Sessions  sessionUseCases
	MCP       mcpUseCases
	Approvals approvalUseCases
	Models    modelUseCases
	Tools     toolUseCases
	Runs      runUseCases
	Queries   queryUseCases
	Usage     usageUseCases
	Feedback  feedbackUseCases

	FileChanges notificationSource[workspaceapp.FileChangeNotice]

	// ServerInfo identifies this runtime on the wire. Name and Version receive
	// development defaults when absent.
	ServerInfo protocol.ServerInfo
	// IdempotencyLimits is the replay window enforced for command retries.
	IdempotencyLimits protocol.IdempotencyLimits

	Schedules      scheduleManagementUseCases
	ScheduleFiring scheduleFiringUseCases
	Invalidations  notificationSource[invalidation.Notice]

	// Goals exposes the autonomous Goal use cases. nil
	// makes goals.* report capability_not_negotiated.
	Goals goalUseCases

	// AgentMemory is the HITL review use-case surface over the agent's
	// self-maintained memory (agentMemory.*). nil makes agentMemory.* report
	// capability_not_negotiated.
	AgentMemory agentMemoryUseCases

	WorkspaceFiles         workspaceFileUseCases
	WorkspaceVCS           workspaceVCSUseCases
	WorkspaceDiscovery     workspaceDiscoveryUseCases
	WorkspaceKnowledge     workspaceKnowledgeUseCases
	WorkspaceSkills        workspaceSkillUseCases
	WorkspaceHooks         workspaceHookUseCases
	WorkspaceWatch         workspaceWatchUseCases
	WorkspaceAuthoredWatch workspaceAuthoredWatchUseCases

	Codebase codebaseUseCases

	GitAvailable bool
	PlanEnabled  bool
}

// Server is the operation.Service implementation exposed via [New].
type Server struct {
	serverInfo protocol.ServerInfo

	sessions       sessionUseCases
	mcp            mcpUseCases
	approvals      approvalUseCases
	models         modelUseCases
	tools          toolUseCases
	codebase       codebaseUseCases
	runs           runUseCases
	queries        queryUseCases
	usage          usageUseCases
	feedback       feedbackUseCases
	schedules      scheduleManagementUseCases
	scheduleFiring scheduleFiringUseCases

	goals       goalUseCases
	agentMemory agentMemoryUseCases

	workspaceFiles         workspaceFileUseCases
	workspaceVCS           workspaceVCSUseCases
	workspaceDiscovery     workspaceDiscoveryUseCases
	workspaceKnowledge     workspaceKnowledgeUseCases
	workspaceSkills        workspaceSkillUseCases
	workspaceHooks         workspaceHookUseCases
	workspaceWatch         workspaceWatchUseCases
	workspaceAuthoredWatch workspaceAuthoredWatchUseCases

	features featureAvailability

	replay                   protocol.RunReplayLimits
	idempotency              protocol.IdempotencyLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits

	// workspaceHub fans non-run change signals out to
	// runtime.subscribe streams (AUX_API §3). It is ephemeral, lossy, and scoped
	// to this process; run streams have their own durable replay contract.
	workspaceHub *workspaceHub
}

// featureAvailability is the small closed set of optional runtime facts that
// shape both capability discovery and delivery gates. Construction derives it
// once; handlers do not rediscover availability by attempting a call.
type featureAvailability struct {
	knowledge   bool
	git         bool
	fileWatch   bool
	plan        bool
	goals       bool
	agentMemory bool
	schedules   bool
	codebase    bool
}

type notificationSource[T any] interface {
	Observe(sink func(T))
}

// Close rejects new runtime subscriptions. Existing streams retain their
// request-owned lifetime. It is safe to call repeatedly.
func (s *Server) Close() {
	if s == nil {
		return
	}
	if s.workspaceHub != nil {
		s.workspaceHub.closeAdmissions()
	}
}

// New builds a Server from its required application use cases.
func New(cfg Config) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withServerInfoDefaults()
	facts, err := deriveContractFacts(cfg)
	if err != nil {
		return nil, err
	}
	server := newServer(cfg, facts)
	server.observeNotificationSources(cfg)
	return server, nil
}

func (cfg Config) validate() error {
	for _, dependency := range []struct {
		name      string
		available bool
	}{
		{name: "Sessions", available: cfg.Sessions != nil},
		{name: "MCP", available: cfg.MCP != nil},
		{name: "Approvals", available: cfg.Approvals != nil},
		{name: "Models", available: cfg.Models != nil},
		{name: "Tools", available: cfg.Tools != nil},
		{name: "Runs", available: cfg.Runs != nil},
		{name: "Queries", available: cfg.Queries != nil},
		{name: "Usage", available: cfg.Usage != nil},
		{name: "Feedback", available: cfg.Feedback != nil},
		{name: "Schedules", available: cfg.Schedules != nil},
		{name: "ScheduleFiring", available: cfg.ScheduleFiring != nil},
		{name: "Codebase", available: cfg.Codebase != nil},
	} {
		if !dependency.available {
			return fmt.Errorf("server: %s is required", dependency.name)
		}
	}
	if cfg.WorkspaceFiles == nil || cfg.WorkspaceVCS == nil ||
		cfg.WorkspaceDiscovery == nil || cfg.WorkspaceKnowledge == nil || cfg.WorkspaceSkills == nil ||
		cfg.WorkspaceHooks == nil || cfg.WorkspaceWatch == nil {
		return errors.New("server: workspace use cases are required")
	}
	if cfg.WorkspaceAuthoredWatch == nil {
		return errors.New("server: authored workspace observation is required")
	}
	return nil
}

func (cfg Config) withServerInfoDefaults() Config {
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "runtime"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0-dev"
	}
	return cfg
}

type contractFacts struct {
	features                 featureAvailability
	replay                   protocol.RunReplayLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits
}

func deriveContractFacts(cfg Config) (contractFacts, error) {
	facts := contractFacts{
		features: featureAvailability{
			knowledge:   cfg.WorkspaceKnowledge.Available(),
			git:         cfg.GitAvailable,
			fileWatch:   cfg.WorkspaceWatch.Available(),
			plan:        cfg.PlanEnabled,
			goals:       cfg.Goals != nil,
			agentMemory: cfg.AgentMemory != nil && cfg.AgentMemory.Available(),
			schedules:   cfg.Schedules.Available() && cfg.ScheduleFiring.Available(),
			codebase:    cfg.Codebase.Available(),
		},
		replay: replayLimitsFrom(cfg.Runs),
		mcpAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{
			RetentionSeconds: int(cfg.MCP.AuthorizationAttemptRetention().Seconds()),
		},
	}
	for _, wireShape := range []struct {
		label string
		value any
	}{
		{label: "Runs returned invalid replay retention", value: facts.replay},
		{label: "IdempotencyLimits is invalid", value: cfg.IdempotencyLimits},
		{label: "MCP returned invalid authorization attempt retention", value: facts.mcpAuthorizationAttempts},
	} {
		if err := protocol.ValidateWireTree(wireShape.value); err != nil {
			return contractFacts{}, fmt.Errorf("server: %s: %w", wireShape.label, err)
		}
	}
	return facts, nil
}

func newServer(cfg Config, facts contractFacts) *Server {
	return &Server{
		sessions:                 cfg.Sessions,
		mcp:                      cfg.MCP,
		approvals:                cfg.Approvals,
		models:                   cfg.Models,
		tools:                    cfg.Tools,
		codebase:                 cfg.Codebase,
		runs:                     cfg.Runs,
		queries:                  cfg.Queries,
		usage:                    cfg.Usage,
		feedback:                 cfg.Feedback,
		serverInfo:               cfg.ServerInfo,
		workspaceHub:             newWorkspaceHub(),
		schedules:                cfg.Schedules,
		scheduleFiring:           cfg.ScheduleFiring,
		goals:                    cfg.Goals,
		agentMemory:              cfg.AgentMemory,
		workspaceFiles:           cfg.WorkspaceFiles,
		workspaceVCS:             cfg.WorkspaceVCS,
		workspaceDiscovery:       cfg.WorkspaceDiscovery,
		workspaceKnowledge:       cfg.WorkspaceKnowledge,
		workspaceSkills:          cfg.WorkspaceSkills,
		workspaceHooks:           cfg.WorkspaceHooks,
		workspaceWatch:           cfg.WorkspaceWatch,
		workspaceAuthoredWatch:   cfg.WorkspaceAuthoredWatch,
		features:                 facts.features,
		replay:                   facts.replay,
		idempotency:              cfg.IdempotencyLimits,
		mcpAuthorizationAttempts: facts.mcpAuthorizationAttempts,
	}
}

func (s *Server) observeNotificationSources(cfg Config) {
	if cfg.FileChanges != nil {
		s.observeFileChanges(cfg.FileChanges)
	}
	if cfg.Invalidations != nil {
		s.observeInvalidations(cfg.Invalidations)
	}
}

// capabilities returns this Server's capability snapshot (API.md §9). Its
// optional keys come from the same immutable composition facts that handlers
// use for their capability gates.
func (s *Server) capabilities() protocol.ServerCapabilities {
	return capabilitiesFor(s.features, s.replay, s.idempotency, s.mcpAuthorizationAttempts)
}

// replayLimitsFrom captures the replay window the Runs use case enforces.
//
// The scope is named here because "which buffer a cursor can reach into" is wire
// vocabulary, and it holds by construction rather than by convention: a Journal
// is created per segment, and every cursor it mints carries that segment and this
// process — so a cursor from another segment is refused as invalid and one from
// another process as unavailable. Making those two refusals happen is what checks
// this claim.
func replayLimitsFrom(useCases runUseCases) protocol.RunReplayLimits {
	retention := useCases.ReplayRetention()
	return protocol.RunReplayLimits{
		Scope:     protocol.ReplayScopeRuntimeInstanceRootSegment,
		MaxEvents: retention.MaxEvents,
		MaxBytes:  retention.MaxBytes,
	}
}

// capabilitiesFor builds the advertised contract from actual composition. A
// capability is never inferred from an RPC error; discovery and gating share
// the same facts so an advertised feature is callable and a disabled feature
// is absent before the client issues a request.
func capabilitiesFor(
	features featureAvailability,
	replay protocol.RunReplayLimits,
	idempotency protocol.IdempotencyLimits,
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits,
) protocol.ServerCapabilities {
	return protocol.ServerCapabilities{
		RunEvents: []protocol.StreamEventType{
			protocol.StreamSegmentStarted,
			protocol.StreamSegmentProgress,
			protocol.StreamSegmentFinished,
			protocol.StreamItemStarted,
			protocol.StreamItemDelta,
			protocol.StreamItemCompleted,
			protocol.StreamStateSnapshot,
		},
		// The subscribable topics, read from the one closed list the subscribe request
		// is validated against. A second list here is how discovery comes to offer a
		// topic the runtime then refuses.
		RuntimeTopics: protocol.RuntimeTopics(),
		// Only the state keys THIS build both writes and can serve a cold read for: a
		// client builds a projection for an advertised key, and a key it could not
		// recover would leave that projection stale with no way back.
		StateSnapshots: advertisedStateSnapshots(features.plan),
		// The two bounds a client cannot discover by trying: what a reconnect can expect
		// to get back, and how wide one subscription may be.
		Limits: protocol.RuntimeLimits{
			Idempotency: idempotency,
			// No process-wide run cap is enforced, so maxConcurrentRuns stays absent
			// rather than advertising a limit the admission layer does not own.
			RunReplay:                replay,
			MCPAuthorizationAttempts: mcpAuthorizationAttempts,
			RuntimeSubscription: protocol.SubscriptionLimits{
				MaxTopics: protocol.MaxSubscriptionTopics, MaxWatches: protocol.MaxSubscriptionWatches,
			},
		},
		// The streaming methods, read from the registry that routes them. A
		// hand-kept list here would be a second author of "which calls stream" —
		// and the one clients trust, since this is what discovery advertises (§9).
		StreamingMethods: operation.Contract().StreamMethods(),
		// Open features map (§9): a client treats an absent key as off. This is the
		// one composition fact per key — whether THIS build offers it — joined with
		// the feature's own published facts (stability, opt-in, whether it reshapes
		// the run protocol), which come from protocol's registry. Advertising them
		// here by hand would let discovery promise a negotiation the runtime does
		// not perform.
		Features: advertisedFeatures(map[string]bool{
			protocol.FeatureReasoning: true,
			protocol.FeatureMCP:       true,
			protocol.FeatureKnowledge: features.knowledge,
			protocol.FeatureSkills:    true,
			protocol.FeatureGit:       features.git,
			protocol.FeatureFileWatch: features.fileWatch,
			protocol.FeatureLSP:       true,

			protocol.FeatureSessionExport: true,
			// File checkpoints (restoreType on rollback) ride the shadow-git
			// store, which needs the git binary — same gate as the git feature.
			protocol.FeatureCheckpoints: features.git,
			protocol.FeatureMultimodal:  true,
			protocol.FeatureRelocate:    true,
			protocol.FeaturePlan:        features.plan,
			protocol.FeatureCompaction:  true,
			protocol.FeatureGoals:       features.goals,
			protocol.FeatureAgentMemory: features.agentMemory,
			protocol.FeatureSchedules:   features.schedules,
			protocol.FeatureCodebase:    features.codebase,
			protocol.FeatureSubagents:   true,
			protocol.FeatureClientTools: false,
		}),
	}
}

// advertisedStateSnapshots publishes the state keys this composition actually serves.
// A key is advertised only when its feature is on: the registry says a key exists and
// names its cold read, but whether THIS build writes it is a composition fact, and a
// client that built a projection for a key nothing writes would hold an empty value it
// could never explain.
//
// The registry's own scope and writer travel with each entry, unchanged. An SDK reads
// them to pick its reducer identity — a session-scoped key is one value per session,
// not one per run — instead of assuming every state belongs to the current run.
func advertisedStateSnapshots(plan bool) []protocol.StateSnapshotCapability {
	enabled := map[string]bool{protocol.FeaturePlan: plan}
	out := make([]protocol.StateSnapshotCapability, 0, len(dispatch.WireShapes().StateKeys()))
	for _, key := range dispatch.WireShapes().StateKeys() {
		if !enabled[key.Feature] {
			continue
		}
		out = append(out, protocol.StateSnapshotCapability{
			Key: protocol.StateSnapshotType(key.Key), RecoveryMethod: key.RecoveryMethod,
			Scope: protocol.StateSnapshotScope(key.Scope), Writer: protocol.StateSnapshotWriter(key.Writer),
		})
	}
	return out
}

// advertisedFeatures joins each published feature with this composition's answer
// to "do we offer it". Every key in the vocabulary is advertised — a client reads
// `enabled:false` and hides the surface, which is more useful than an absent key it
// has to guess about — and a build fact for a key no vocabulary defines is a
// programming error, since a client could never ask about it.
func advertisedFeatures(enabled map[string]bool) map[string]protocol.FeatureCapability {
	for key := range enabled {
		if _, published := protocol.LookupFeature(key); !published {
			panic("server: composition advertises unpublished feature " + key)
		}
	}
	features := protocol.Features()
	out := make(map[string]protocol.FeatureCapability, len(features))
	for _, feature := range features {
		out[feature.Key] = protocol.FeatureCapability{
			Enabled:               enabled[feature.Key],
			Stability:             feature.Stability,
			ClientOptIn:           feature.ClientOptIn,
			RequiredByRunProtocol: feature.RequiredByRunProtocol,
		}
	}
	return out
}

// ─── helpers ────────────────────────────────────────────────────────

// capabilityNotNegotiated marks a protocol method that exists in the contract
// but isn't backed on this build. Maps to capability_not_negotiated (API.md §8.2)
// — consistent with the feature flag advertised through discovery.
func capabilityNotNegotiated(method string) error {
	return fmt.Errorf("%w: %s", protocol.ErrCapabilityNotNeg, method)
}
