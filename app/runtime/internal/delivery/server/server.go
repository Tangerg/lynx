// Package server realizes protocol.Runtime on top of the application
// coordinators + domain layer (API.md §0 model: Session → Run → Item). It's
// the single place where the JSON-RPC method table (delivery/dispatch) and
// the runtime's chat / session / tool / memory stores meet.
//
// Every method is wired to an application coordinator or an explicit adapter;
// discovery is generated from that same wiring so the advertised contract and
// callable surface cannot drift.
package server

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// Config bundles construction inputs.
type Config struct {
	// Sessions is the application coordinator for the session/run lifecycle
	// write-sets and single-writer admission (rollback / delete cascade / fork /
	// restore / resume / working-tree gates). Required — the delivery layer drives
	// every lifecycle mutation through it.
	Sessions sessionUseCases

	// Integrations is the application coordinator for the runtime's MCP
	// integration surface (server registry / live pool / tool policy). Required —
	// the delivery mcp.* handlers drive it directly.
	Integrations integrationUseCases

	// Approvals is the application coordinator for the tool-permission stance +
	// approval rules. Required — the approval.* settings handlers drive it.
	Approvals approvalUseCases

	// Models is the application coordinator for provider + model configuration
	// (providers.* / models.* / the default provider+model). Required — the
	// provider/model settings handlers + the capability snapshot drive it.
	Models modelUseCases

	// Tools is the application coordinator for the diagnostic tool registry
	// (tools.list / tools.invoke). Required — the tools.* handlers drive it.
	Tools toolUseCases

	// Coordinator owns the run lifecycle (admission / journal / pump / cancel),
	// built + owned by the composition root (bootstrap.Host). Required — delivery
	// drives it as a use-case surface but never constructs or closes it (§11.1).
	Coordinator runUseCases

	// Queries is the application read coordinator for a session's durable
	// execution record (transcript / interrupts). Required — the items.list and
	// interrupts.list handlers drive it.
	Queries queryUseCases

	// Usage folds durable run history into session and aggregate metering
	// reports. Required — delivery only projects its result onto usage.*.
	Usage usageUseCases

	// Feedback records feedback.create quality signals through the application's
	// durable feedback use case. Required: this protocol method must never ack a
	// discarded signal.
	Feedback feedbackUseCases

	// FileChanges is the composition-root bridge the run pump publishes live
	// file-change nudges through; the Server installs a consumer that maps them to
	// wire workspace events on the hub. Required in production; nil in tests that
	// don't exercise the workspace stream.
	FileChanges Source[runs.FileChange]

	// MCPStatus is the composition-root bridge the integrations coordinator
	// publishes MCP connection transitions through; the Server installs a consumer
	// that maps them to mcp.serverChanged workspace events. Required in production;
	// nil in tests that don't exercise the MCP status stream.
	MCPStatus Source[integrations.MCPServerStatus]

	// SkillChanges is the composition-root bridge the workspace skills use case
	// publishes after a committed library change; the Server maps the nudge to a
	// skills.changed workspace event. Nil in tests that do not exercise it.
	SkillChanges Source[struct{}]

	// ServerInfo identifies this process on the wire. Defaults to
	// {Name: "runtime", Version: "0.0.0-dev"} when zero — a vendor-neutral
	// name, since the protocol is consumed by arbitrary clients and the
	// rpc/protocol package is the codegen SSOT for other languages.
	ServerInfo protocol.ServerInfo

	// Schedules manages cron-triggered headless runs. Bootstrap supplies an
	// explicit disabled coordinator when the capability is not negotiated.
	Schedules scheduleManagementUseCases

	// ScheduleFiring starts an accepted schedule without coupling Delivery to
	// worker construction or the Runs coordinator.
	ScheduleFiring scheduleFiringUseCases

	// Changes carries every committed session / run / interrupt / goal / state
	// change from the use case that committed it; the Server maps each to the one
	// invalidation signal for its topic (§7.3). Required in production: without it a
	// second window learns nothing about work it is not itself driving. Nil in tests
	// that don't exercise the change stream.
	Changes Source[change.Notice]

	// ScheduleFires carries accepted scheduled-run notifications from the
	// composition root. Delivery projects them to workspace events; it does not
	// construct or run the scheduler.
	ScheduleFires Source[string]

	// Goals is the autonomous-execution loop driver (goals.* — Goal mode). nil
	// makes goals.* report capability_not_negotiated.
	Goals goalRunner

	// AgentMemory is the HITL review use-case surface over the agent's
	// self-maintained memory (agentMemory.*). nil makes agentMemory.* report
	// capability_not_negotiated.
	AgentMemory agentMemoryUseCases

	// Workspace capabilities are independent application use cases. Delivery
	// depends on each narrow consumer port, never a catch-all workspace facade.
	WorkspaceFiles     workspaceFileUseCases
	WorkspaceVCS       workspaceVCSUseCases
	WorkspaceDiscovery workspaceDiscoveryUseCases
	WorkspaceKnowledge workspaceKnowledgeUseCases
	WorkspaceSkills    workspaceSkillUseCases
	WorkspaceHooks     workspaceHookUseCases
	WorkspaceWatch     workspaceWatchUseCases

	// Codebase is the application coordinator for the @codebase semantic index
	// (codebase.search / status / reindex). Bootstrap supplies its unavailable
	// coordinator when no index is configured.
	Codebase codebaseUseCases

	// GitAvailable is the Bootstrap-probed Git capability snapshot. Delivery
	// projects this static environment fact; it never probes the process itself.
	GitAvailable bool

	// PlanEnabled is the composition-root fact for set_plan. Plan persistence
	// is consumed by execution, not a delivery handler, so it is supplied here
	// rather than inferred from an unrelated coordinator.
	PlanEnabled bool
}

// Server is the protocol.Runtime implementation exposed via [New].
type Server struct {
	serverInfo protocol.ServerInfo

	// sessions owns the session/run lifecycle write-sets and single-writer
	// admission gates (rollback / delete cascade / fork / restore / resume /
	// working-tree). Injected by the composition root; never nil after New.
	sessions sessionUseCases

	// integrations owns the MCP integration use cases (server registry / live pool
	// / tool policy). Injected by the composition root; never nil after New.
	integrations integrationUseCases

	// approvals owns the tool-permission stance + approval-rule use cases. Injected
	// by the composition root; never nil after New.
	approvals approvalUseCases

	// models owns provider + model configuration (registry / catalog / roles /
	// defaults). Injected by the composition root; never nil after New.
	models modelUseCases

	// tools owns the diagnostic tool-registry read/invoke use cases. Injected by
	// the composition root; never nil after New.
	tools toolUseCases

	// codebase owns the @codebase semantic-index use cases (search / status /
	// reindex). Injected by the composition root; never nil after New.
	codebase codebaseUseCases

	// coordinator owns the run lifecycle — admission, the per-run event Journal,
	// the segment pumps, cancel. Built + owned by the composition root
	// (bootstrap.Host); delivery drives it as a use-case surface and never closes
	// it (§11.1). Injected by New; never nil after New.
	coordinator runUseCases

	// queries is the application read coordinator for a session's durable
	// execution record (transcript / interrupts). Injected by New.
	queries queryUseCases

	// usage owns durable metering aggregation. Delivery only maps its neutral
	// result values to the protocol response.
	usage usageUseCases

	// feedback owns the feedback.create durable write use case.
	feedback feedbackUseCases

	// schedules owns editable cron-triggered headless-run management.
	schedules scheduleManagementUseCases
	// scheduleFiring owns accepted manual fires; worker lifetime remains outside
	// Delivery in the command host.
	scheduleFiring scheduleFiringUseCases

	// goals drives the autonomous-execution loop (goals.* — Goal mode). Never nil
	// after New (a disabled stub when Goal mode is off).
	goals goalRunner

	// agentMemory is the HITL review use-case surface over agent memory
	// (agentMemory.*). nil means the capability was not negotiated.
	agentMemory agentMemoryUseCases

	// Workspace use cases are intentionally separate bounded capabilities.
	workspaceFiles     workspaceFileUseCases
	workspaceVCS       workspaceVCSUseCases
	workspaceDiscovery workspaceDiscoveryUseCases
	workspaceKnowledge workspaceKnowledgeUseCases
	workspaceSkills    workspaceSkillUseCases
	workspaceHooks     workspaceHookUseCases
	workspaceWatch     workspaceWatchUseCases

	features featureAvailability

	// replay is the advertised replay window, captured and validated from the
	// required run coordinator at construction.
	replay                   protocol.RunReplayLimits
	mcpAuthorizationAttempts protocol.MCPAuthorizationAttemptLimits

	// wsHub fans non-run change signals (files/skills/mcp/schedule/session) out to
	// runtime.subscribe streams (AUX_API §3). Ephemeral, lossy, connection-
	// scoped — distinct from the durable per-run hubs.
	wsHub *workspaceHub
}

// featureAvailability is the small closed set of optional runtime facts that
// shape both capability discovery and delivery gates. It is derived once from
// actual composition; no handler probes a disabled implementation to discover
// whether it may be called.
type featureAvailability struct {
	memory      bool
	git         bool
	fileWatch   bool
	plan        bool
	goals       bool
	agentMemory bool
	schedules   bool
	codebase    bool
}

// Source is the delivery-side, observe-only view of a composition-root signal
// bridge (component/signal.Signal[T]). The Server installs exactly one observer
// that maps each published payload to a wire workspace event on the hub; the
// concrete notifier — and its Publish side — is owned by the Host, which hands
// the publish end to the producer (run effects / integrations coordinator /
// skills use case / scheduler). Narrowing to Observe keeps Publish off the
// delivery surface. The per-bridge semantics live on the Config fields.
type Source[T any] interface {
	Observe(sink func(T))
}

// Close marks the Server shut down so new workspace subscriptions are rejected;
// in-flight streams end with their request contexts, and the run coordinator's
// pumps are joined by the Host, not here (§11.1). Safe to call repeatedly.
func (s *Server) Close() {
	if s == nil {
		return
	}
	if s.wsHub != nil {
		s.wsHub.closeAdmissions()
	}
}

// New builds a Server. Returns an error when a required coordinator is nil. The
// concrete *Server is returned because it satisfies [protocol.Runtime] and owns
// delivery-local workspace subscriptions.
func New(cfg Config) (*Server, error) {
	if cfg.Sessions == nil {
		return nil, errors.New("server: Sessions is required")
	}
	if cfg.Integrations == nil {
		return nil, errors.New("server: Integrations is required")
	}
	if cfg.Approvals == nil {
		return nil, errors.New("server: Approvals is required")
	}
	if cfg.Models == nil {
		return nil, errors.New("server: Models is required")
	}
	if cfg.Tools == nil {
		return nil, errors.New("server: Tools is required")
	}
	if cfg.ServerInfo.Name == "" {
		cfg.ServerInfo.Name = "runtime"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0-dev"
	}
	if cfg.Coordinator == nil {
		return nil, errors.New("server: Coordinator is required")
	}
	if cfg.Queries == nil {
		return nil, errors.New("server: Queries is required")
	}
	if cfg.Usage == nil {
		return nil, errors.New("server: Usage is required")
	}
	if cfg.Feedback == nil {
		return nil, errors.New("server: Feedback is required")
	}
	if cfg.Schedules == nil {
		return nil, errors.New("server: Schedules is required")
	}
	if cfg.ScheduleFiring == nil {
		return nil, errors.New("server: ScheduleFiring is required")
	}
	if cfg.WorkspaceFiles == nil || cfg.WorkspaceVCS == nil ||
		cfg.WorkspaceDiscovery == nil || cfg.WorkspaceKnowledge == nil || cfg.WorkspaceSkills == nil ||
		cfg.WorkspaceHooks == nil || cfg.WorkspaceWatch == nil {
		return nil, errors.New("server: workspace use cases are required")
	}
	if cfg.Codebase == nil {
		return nil, errors.New("server: Codebase is required")
	}
	features := featureAvailability{
		memory:      cfg.WorkspaceKnowledge.HasMemory(),
		git:         cfg.GitAvailable,
		fileWatch:   cfg.WorkspaceWatch.HasFileWatch(),
		plan:        cfg.PlanEnabled,
		goals:       cfg.Goals != nil,
		agentMemory: cfg.AgentMemory != nil && cfg.AgentMemory.Available(),
		schedules:   cfg.Schedules.Available() && cfg.ScheduleFiring.Available(),
		codebase:    cfg.Codebase.Available(),
	}
	replay := replayLimitsFrom(cfg.Coordinator)
	if err := protocol.ValidateWireTree(replay); err != nil {
		return nil, fmt.Errorf("server: coordinator returned invalid replay retention: %w", err)
	}
	mcpAuthorizationAttempts := protocol.MCPAuthorizationAttemptLimits{
		RetentionSeconds: int(cfg.Integrations.MCPAuthorizationAttemptRetention().Seconds()),
	}
	if err := protocol.ValidateWireTree(mcpAuthorizationAttempts); err != nil {
		return nil, fmt.Errorf("server: integrations returned invalid MCP authorization attempt retention: %w", err)
	}
	srv := &Server{
		sessions:                 cfg.Sessions,
		integrations:             cfg.Integrations,
		approvals:                cfg.Approvals,
		models:                   cfg.Models,
		tools:                    cfg.Tools,
		codebase:                 cfg.Codebase,
		coordinator:              cfg.Coordinator,
		queries:                  cfg.Queries,
		usage:                    cfg.Usage,
		feedback:                 cfg.Feedback,
		serverInfo:               cfg.ServerInfo,
		wsHub:                    newWorkspaceHub(),
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
		features:                 features,
		replay:                   replay,
		mcpAuthorizationAttempts: mcpAuthorizationAttempts,
	}
	// The run pump publishes live file-change nudges through the composition-root
	// bridge; the Server maps each to a wire workspace event on its hub. This is
	// the seam that lets the coordinator be built in the Host (§11.1/§13.2) — its
	// effects need a publish sink, but the hub is constructed here in delivery.
	if cfg.FileChanges != nil {
		srv.wsHub.observe(cfg.FileChanges)
	}
	// MCP reconnect/authorize run fire-and-forget in the integrations coordinator;
	// their connecting → settled transitions reach the workspace hub through this
	// bridge, mapped to mcp.serverChanged frames.
	if cfg.MCPStatus != nil {
		srv.observeMCPStatus(cfg.MCPStatus)
	}
	if cfg.SkillChanges != nil {
		srv.observeSkillChanges(cfg.SkillChanges)
	}
	if cfg.ScheduleFires != nil {
		srv.observeScheduleFires(cfg.ScheduleFires)
	}
	if cfg.Changes != nil {
		srv.observeChanges(cfg.Changes)
	}
	return srv, nil
}

// Capabilities returns this Server's capability snapshot (API.md §9). Its
// optional keys come from the same immutable composition facts that handlers
// use for their capability gates.
func (s *Server) Capabilities() protocol.ServerCapabilities {
	return capabilitiesFor(s.features, s.replay, s.mcpAuthorizationAttempts)
}

// replayLimitsFrom captures the replay window the run coordinator enforces, at
// construction, the way every other composition fact is captured. New requires a
// coordinator, so an absent replay promise is not a valid server composition.
//
// The scope is named here because "which buffer a cursor can reach into" is wire
// vocabulary, and it holds by construction rather than by convention: a Journal
// is created per segment, and every cursor it mints carries that segment and this
// process — so a cursor from another segment is refused as invalid and one from
// another process as unavailable. Making those two refusals happen is what checks
// this claim.
func replayLimitsFrom(coordinator runUseCases) protocol.RunReplayLimits {
	retention := coordinator.ReplayRetention()
	return protocol.RunReplayLimits{
		Scope:     protocol.ReplayScopeProcessRootSegment,
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
			Idempotency: protocol.IdempotencyLimits{
				RetentionSeconds: int(idempotency.Retention.Seconds()),
			},
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
		StreamingMethods: dispatch.Contract().StreamMethods(),
		// Open features map (§9): a client treats an absent key as off. This is the
		// one composition fact per key — whether THIS build offers it — joined with
		// the feature's own published facts (stability, opt-in, whether it reshapes
		// the run protocol), which come from protocol's registry. Advertising them
		// here by hand would let discovery promise a negotiation the runtime does
		// not perform.
		Features: advertisedFeatures(map[string]bool{
			protocol.FeatureReasoning: true,
			protocol.FeatureMCP:       true,
			protocol.FeatureMemory:    features.memory,
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
