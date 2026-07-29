package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerWorkspace(r *Registry) {
	// The git reads are NOT capability-gated: features.git=false tells the client
	// to hide the panel and not call, and a non-repo cwd is vcs_unavailable — three
	// distinct answers the contract keeps distinct (AUX_API §2).
	Unary(r, MethodMeta{
		Name: "workspace.listFileChanges",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
		return d.api.ListWorkspaceFileChanges(ctx, in)
	})

	Unary(r, MethodMeta{
		Name: "workspace.getDiff",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrVcsUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetDiffRequest) (*protocol.Diff, error) {
		return d.api.GetWorkspaceDiff(ctx, in)
	})

	Unary(r, MethodMeta{
		Name: "workspace.getFileHead",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
		return d.api.GetWorkspaceFileHead(ctx, in)
	})

	Unary(r, MethodMeta{
		Name: "workspace.grep",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
		return d.api.GrepWorkspace(ctx, in)
	})

	Unary(r, MethodMeta{
		Name: "workspace.listFiles",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
		return d.api.ListWorkspaceFiles(ctx, in)
	})

	Unary(r, MethodMeta{
		Name: "workspace.readFile",
		Errors: []string{
			protocol.ErrCwdUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
		return d.api.ReadWorkspaceFile(ctx, in)
	})

	Unary(r, MethodMeta{Name: "workspace.listProjects", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Project], error) {
			return d.api.ListWorkspaceProjects(ctx, in)
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
	Stream(r, MethodMeta{
		Name: "runtime.subscribe",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "watches", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureFileWatch},
		}},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
		return d.api.SubscribeRuntime(ctx, in)
	}, runtimeEventFramer)
}

func registerSkills(r *Registry) {
	Unary(r, MethodMeta{
		Name:            "skills.discovered.list",
		Errors:          []string{protocol.ErrCwdUnavailable.Error()},
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
		return d.api.ListDiscoveredSkills(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "skills.library.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.ManagedSkill], error) {
		return d.api.ListManagedSkills(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "skills.library.archive",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.ArchiveSkill(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "skills.library.restore",
		Idempotency:     IdempotencyReplayResponse,
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SkillNameRequest) error {
		return d.api.RestoreSkill(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:            "skills.drafts.list",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.SkillDraft], error) {
		return d.api.ListSkillDrafts(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "skills.drafts.promote",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SkillDraftRef) error {
		return d.api.PromoteSkillDraft(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:            "skills.drafts.reject",
		CapabilityRules: requires(protocol.FeatureSkills),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SkillDraftRef) error {
		return d.api.RejectSkillDraft(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:      "recipes.list",
		Errors:    []string{protocol.ErrCwdUnavailable.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Recipe], error) {
		return d.api.ListRecipes(ctx, in)
	})

	Unary(r, MethodMeta{
		Name:      "agentDocs.list",
		Errors:    []string{protocol.ErrCwdUnavailable.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return d.api.ListAgentDocs(ctx, in)
	})
}

func registerHooks(r *Registry) {
	Unary(r, MethodMeta{
		Name:      "hooks.list",
		Errors:    []string{protocol.ErrCwdUnavailable.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return d.api.ListHooks(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:        "hooks.setTrust",
		Idempotency: IdempotencyReplayResponse,
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SetHookTrustRequest) error {
		return d.api.SetHookTrust(ctx, in)
	})
}
