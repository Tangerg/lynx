package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerWorkspace(r *Registry) {
	Query(r, MethodMeta{
		Name:      "workspaces.resolve",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
		return d.api.ResolveWorkspace(ctx, in)
	})

	Query(r, MethodMeta{Name: "workspaces.list", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.WorkspaceSummary], error) {
			return d.api.ListWorkspaces(ctx)
		})

	// Git reads require the advertised capability. Once negotiated, a path that is
	// not a repository is the distinct vcs_unavailable domain answer.
	Query(r, MethodMeta{
		Name: "workspace.changes.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
		return d.api.ListWorkspaceFileChanges(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.diff.get",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		CapabilityRules: requires(protocol.FeatureGit),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
		return d.api.GetWorkspaceDiff(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.head",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
		return d.api.GetWorkspaceFileHead(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.search",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
		return d.api.GrepWorkspace(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.list",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
		return d.api.ListWorkspaceFiles(ctx, in)
	})

	Query(r, MethodMeta{
		Name: "workspace.files.read",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
		return d.api.ReadWorkspaceFile(ctx, in)
	})
}

func registerRuntimeSubscription(r *Registry) {
	// Only a subscription that registers watches needs features.fileWatch —
	// subscribing for the global topics is always available (§7.1). The condition
	// treats `watches: []` as "no watches", so an explicitly empty list and an absent
	// one behave alike.
	//
	// A topic this build does not advertise is refused with capability_not_negotiated
	// by the handler, which is the only place that knows the composition's answer.
	Subscription(r, MethodMeta{
		Name: "runtime.subscribe",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "watches", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureFileWatch},
		}},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
		return d.api.SubscribeRuntime(ctx, in)
	}, runtimeEventFramer)
}

func registerSkills(r *Registry) {
	Query(r, MethodMeta{
		Name:            "skills.discovered.list",
		Errors:          []string{protocol.ErrWorkspaceUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Skill], error) {
		return d.api.ListDiscoveredSkills(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "skills.library.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ManagedSkill], error) {
		return d.api.ListManagedSkills(ctx)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.library.archive",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.ArchiveSkill(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.library.restore",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.RestoreSkill(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "skills.proposals.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
		return d.api.ListSkillProposals(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.proposals.approve",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillProposalRef) error {
		return d.api.ApproveSkillProposal(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "skills.proposals.reject",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.SkillProposalRef) error {
		return d.api.RejectSkillProposal(ctx, in)
	})

	Query(r, MethodMeta{
		Name:      "recipes.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.Recipe], error) {
		return d.api.ListRecipes(ctx, in)
	})

	Query(r, MethodMeta{
		Name:      "agentDocs.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return d.api.ListAgentDocs(ctx, in)
	})
}

func registerHooks(r *Registry) {
	Query(r, MethodMeta{
		Name:      "hooks.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return d.api.ListHooks(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:      "hooks.setTrust",
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.SetHookTrustRequest) error {
		return d.api.SetHookTrust(ctx, in)
	})
}
