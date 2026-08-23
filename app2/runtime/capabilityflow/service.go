// Package capabilityflow owns human-authored prompt resources and reviewed
// agent capabilities. Filesystem conventions are kept here, outside the
// protocol and execution engine.
package capabilityflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const maxAuthoredDocumentBytes = 1 << 20

type Store interface {
	ListManagedSkillRecords(context.Context) ([]protocol.ManagedSkill, error)
	PutManagedSkill(context.Context, protocol.ManagedSkill) error
	DeleteManagedSkill(context.Context, string) error
	ListSkillProposalRecords(context.Context, string) ([]protocol.SkillProposal, error)
	GetSkillProposalRecord(context.Context, string, string, string) (protocol.SkillProposal, error)
	PutSkillProposalRecord(context.Context, string, protocol.SkillProposal) error
	DeleteSkillProposalRecord(context.Context, string, string, string) error
	GetProjectHookTrust(context.Context, string) (bool, error)
	SetProjectHookTrust(context.Context, string, bool) error
}

type Resolver interface { Resolve(context.Context, string) (workspacefs.Resolution, error) }
type IDs interface { New(string) (string, error) }

type Service struct {
	store       Store
	resolver    Resolver
	ids         IDs
	home        string
	userRoot    string
	serial      *identityCoordinator
}

func New(store Store, resolver Resolver, ids IDs, home string) (*Service, error) {
	if store == nil || resolver == nil || ids == nil || !filepath.IsAbs(home) {
		return nil, errors.New("capabilityflow: store, resolver, ids and absolute home are required")
	}
	home = filepath.Clean(home)
	if err := prepareUserSkillLibrary(home); err != nil {
		return nil, fmt.Errorf("capabilityflow: prepare user skills: %w", err)
	}
	return &Service{
		store:       store,
		resolver:    resolver,
		ids:         ids,
		home:        home,
		userRoot:    filepath.Join(home, ".lyra"),
		serial:      newIdentityCoordinator(),
	}, nil
}

func (service *Service) ListHooks(ctx context.Context,request protocol.ListHooksRequest)(*protocol.HooksListResult,error){resolved,err:=service.resolve(ctx,&request.Workspace);if err!=nil{return nil,err};root:=resolved.ProjectRoot;if root==""{root=resolved.Workspace.Path()};trusted,err:=service.store.GetProjectHookTrust(ctx,root);if err!=nil{return nil,err};values:=make([]protocol.HookInfo,0);load:=func(path string,scope protocol.HookScope)error{data,err:=os.ReadFile(path);if errors.Is(err,os.ErrNotExist){return nil};if err!=nil{return err};var file struct{Hooks []struct{Event protocol.HookEvent `json:"event"`;Matcher string `json:"matcher"`;Command string `json:"command"`;Inject string `json:"inject"`;TimeoutMillis int `json:"timeoutMillis"`} `json:"hooks"`};if err:=json.Unmarshal(data,&file);err!=nil{return fmt.Errorf("hooks: parse %s: %w",path,err)};for _,wire:=range file.Hooks{if wire.Command==""&&wire.Inject==""||wire.Command!=""&&wire.Inject!=""{return fmt.Errorf("hooks: exactly one of command/inject is required")};values=append(values,protocol.HookInfo{Event:wire.Event,Matcher:wire.Matcher,Command:wire.Command,Inject:wire.Inject,TimeoutMillis:wire.TimeoutMillis,Scope:scope,Source:path,Active:scope==protocol.HookScopeGlobal||trusted})};return nil};if err:=load(filepath.Join(service.userRoot,"hooks.json"),protocol.HookScopeGlobal);err!=nil{return nil,err};for _,dir:=range rootToLeaf(root,resolved.Workspace.Path()){if err:=load(filepath.Join(dir,".lyra","hooks.json"),protocol.HookScopeProject);err!=nil{return nil,err}};return &protocol.HooksListResult{ProjectRoot:root,ProjectTrusted:trusted,Hooks:values},nil}
func (service *Service) SetHookTrust(ctx context.Context,request protocol.SetHookTrustRequest)error{if !filepath.IsAbs(request.ProjectRoot){return fmt.Errorf("%w: project root must be absolute",protocol.ErrInvalidParams)};return service.store.SetProjectHookTrust(ctx,filepath.Clean(request.ProjectRoot),request.Trusted)}

func (service *Service) resolve(ctx context.Context,workspace *protocol.WorkspaceRef)(workspacefs.Resolution,error){requested:="";if workspace!=nil{requested=workspace.Path};resolved,err:=service.resolver.Resolve(ctx,requested);if err!=nil||!resolved.Available{return workspacefs.Resolution{},protocol.ErrWorkspaceUnavailable};return resolved,nil}
func rootToLeaf(root,leaf string)[]string{root=filepath.Clean(root);leaf=filepath.Clean(leaf);values:=[]string{};for current:=leaf;;current=filepath.Dir(current){values=append(values,current);if current==root||filepath.Dir(current)==current{break}};slices.Reverse(values);return values}
