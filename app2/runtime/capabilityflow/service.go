// Package capabilityflow owns human-authored prompt resources and reviewed
// agent capabilities. Filesystem conventions are kept here, outside the
// protocol and execution engine.
package capabilityflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/sqlite"
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
	ListAgentMemoryRecords(context.Context, protocol.AgentMemoryScope, string) ([]protocol.AgentMemoryItem, error)
	GetAgentMemoryRecord(context.Context, string) (protocol.AgentMemoryItem, string, error)
	PutAgentMemoryRecord(context.Context, protocol.AgentMemoryItem, string) error
	DeleteAgentMemoryRecord(context.Context, string) error
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
	mu          sync.Mutex
	skillSerial *skillCoordinator
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
		skillSerial: newSkillCoordinator(),
	}, nil
}

func (service *Service) ListKnowledge(ctx context.Context,query protocol.WorkspaceQuery)(*protocol.Page[protocol.KnowledgeEntry],error){values:=make([]protocol.KnowledgeEntry,0,3);for _,scope:=range []protocol.KnowledgeScope{protocol.KnowledgeScopeCWD,protocol.KnowledgeScopeProjectRoot,protocol.KnowledgeScopeHome}{value,err:=service.GetKnowledge(ctx,protocol.GetKnowledgeRequest{Scope:scope,Workspace:&query.Workspace});if err!=nil{return nil,err};values=append(values,*value)};return protocol.NewPage(values),nil}
func (service *Service) GetKnowledge(ctx context.Context,request protocol.GetKnowledgeRequest)(*protocol.KnowledgeEntry,error){path,err:=service.knowledgePath(ctx,request.Scope,request.Workspace);if err!=nil{return nil,err};data,modified,err:=readOptional(path);if err!=nil{return nil,err};return &protocol.KnowledgeEntry{Scope:request.Scope,Content:string(data),Revision:revision(data),UpdatedAt:modified},nil}
func (service *Service) UpdateKnowledge(ctx context.Context,request protocol.UpdateKnowledgeRequest)(*protocol.KnowledgeEntry,error){if !request.Scope.Valid(){return nil,fmt.Errorf("%w: invalid knowledge scope",protocol.ErrInvalidParams)};path,err:=service.knowledgePath(ctx,request.Scope,request.Workspace);if err!=nil{return nil,err};service.mu.Lock();defer service.mu.Unlock();current,_,err:=readOptional(path);if err!=nil{return nil,err};if revision(current)!=request.ExpectedRevision{return nil,protocol.ErrRevisionConflict};mode:=os.FileMode(0o644);if request.Scope==protocol.KnowledgeScopeHome{mode=0o600};if err:=atomicWrite(path,[]byte(request.Content),mode);err!=nil{return nil,err};info,err:=os.Stat(path);if err!=nil{return nil,err};return &protocol.KnowledgeEntry{Scope:request.Scope,Content:request.Content,Revision:revision([]byte(request.Content)),UpdatedAt:info.ModTime()},nil}

func (service *Service) ListMemory(ctx context.Context,request protocol.AgentMemoryListRequest)(*protocol.AgentMemoryList,error){project,err:=service.memoryProject(ctx,request.Scope,request.Workspace);if err!=nil{return nil,err};values,err:=service.store.ListAgentMemoryRecords(ctx,request.Scope,project);if err!=nil{return nil,err};return &protocol.AgentMemoryList{Items:values},nil}
func (service *Service) AddMemory(ctx context.Context,request protocol.AgentMemoryAddRequest)(*protocol.AgentMemoryItem,error){if strings.TrimSpace(request.Content)==""{return nil,fmt.Errorf("%w: content is required",protocol.ErrInvalidParams)};project,err:=service.memoryProject(ctx,request.Scope,request.Workspace);if err!=nil{return nil,err};id,err:=service.ids.New("mem_");if err!=nil{return nil,err};now:=time.Now().UTC();value:=protocol.AgentMemoryItem{ID:id,Scope:request.Scope,Content:strings.TrimSpace(request.Content),Origin:protocol.AgentMemoryOriginUser,Status:protocol.AgentMemoryStatusActive,CreatedAt:now,UpdatedAt:now};if err:=service.store.PutAgentMemoryRecord(ctx,value,project);err!=nil{return nil,err};return &value,nil}
func (service *Service) ReviewMemory(ctx context.Context,request protocol.AgentMemoryReviewRequest)error{value,project,err:=service.store.GetAgentMemoryRecord(ctx,request.ID);if err!=nil{return protocol.ErrItemNotFound};if value.Status!=protocol.AgentMemoryStatusPending{return fmt.Errorf("%w: memory is not pending",protocol.ErrInvalidParams)};if request.Decision==protocol.AgentMemoryReviewReject{return service.store.DeleteAgentMemoryRecord(ctx,request.ID)};if request.Decision!=protocol.AgentMemoryReviewApprove{return fmt.Errorf("%w: invalid review decision",protocol.ErrInvalidParams)};value.Status=protocol.AgentMemoryStatusActive;value.UpdatedAt=time.Now().UTC();return service.store.PutAgentMemoryRecord(ctx,value,project)}
func (service *Service) UpdateMemory(ctx context.Context,request protocol.AgentMemoryUpdateRequest)(*protocol.AgentMemoryItem,error){value,project,err:=service.store.GetAgentMemoryRecord(ctx,request.ID);if err!=nil{return nil,protocol.ErrItemNotFound};if request.Content!=nil{if strings.TrimSpace(*request.Content)==""{return nil,fmt.Errorf("%w: content is required",protocol.ErrInvalidParams)};value.Content=strings.TrimSpace(*request.Content)};if request.Pinned!=nil{value.Pinned=*request.Pinned};value.UpdatedAt=time.Now().UTC();if err:=service.store.PutAgentMemoryRecord(ctx,value,project);err!=nil{return nil,err};return &value,nil}
func (service *Service) DeleteMemory(ctx context.Context,id string)error{if err:=service.store.DeleteAgentMemoryRecord(ctx,id);err!=nil{if errors.Is(err,sqlite.ErrCapabilityNotFound){return protocol.ErrItemNotFound};return err};return nil}

func (service *Service) ListHooks(ctx context.Context,request protocol.ListHooksRequest)(*protocol.HooksListResult,error){resolved,err:=service.resolve(ctx,&request.Workspace);if err!=nil{return nil,err};root:=resolved.ProjectRoot;if root==""{root=resolved.Workspace.Path()};trusted,err:=service.store.GetProjectHookTrust(ctx,root);if err!=nil{return nil,err};values:=make([]protocol.HookInfo,0);load:=func(path string,scope protocol.HookScope)error{data,err:=os.ReadFile(path);if errors.Is(err,os.ErrNotExist){return nil};if err!=nil{return err};var file struct{Hooks []struct{Event protocol.HookEvent `json:"event"`;Matcher string `json:"matcher"`;Command string `json:"command"`;Inject string `json:"inject"`;TimeoutMillis int `json:"timeoutMillis"`} `json:"hooks"`};if err:=json.Unmarshal(data,&file);err!=nil{return fmt.Errorf("hooks: parse %s: %w",path,err)};for _,wire:=range file.Hooks{if wire.Command==""&&wire.Inject==""||wire.Command!=""&&wire.Inject!=""{return fmt.Errorf("hooks: exactly one of command/inject is required")};values=append(values,protocol.HookInfo{Event:wire.Event,Matcher:wire.Matcher,Command:wire.Command,Inject:wire.Inject,TimeoutMillis:wire.TimeoutMillis,Scope:scope,Source:path,Active:scope==protocol.HookScopeGlobal||trusted})};return nil};if err:=load(filepath.Join(service.userRoot,"hooks.json"),protocol.HookScopeGlobal);err!=nil{return nil,err};for _,dir:=range rootToLeaf(root,resolved.Workspace.Path()){if err:=load(filepath.Join(dir,".lyra","hooks.json"),protocol.HookScopeProject);err!=nil{return nil,err}};return &protocol.HooksListResult{ProjectRoot:root,ProjectTrusted:trusted,Hooks:values},nil}
func (service *Service) SetHookTrust(ctx context.Context,request protocol.SetHookTrustRequest)error{if !filepath.IsAbs(request.ProjectRoot){return fmt.Errorf("%w: project root must be absolute",protocol.ErrInvalidParams)};return service.store.SetProjectHookTrust(ctx,filepath.Clean(request.ProjectRoot),request.Trusted)}

func (service *Service) resolve(ctx context.Context,workspace *protocol.WorkspaceRef)(workspacefs.Resolution,error){requested:="";if workspace!=nil{requested=workspace.Path};resolved,err:=service.resolver.Resolve(ctx,requested);if err!=nil||!resolved.Available{return workspacefs.Resolution{},protocol.ErrWorkspaceUnavailable};return resolved,nil}
func (service *Service) knowledgePath(ctx context.Context,scope protocol.KnowledgeScope,workspace *protocol.WorkspaceRef)(string,error){if !scope.Valid(){return "",fmt.Errorf("%w: invalid knowledge scope",protocol.ErrInvalidParams)};if scope==protocol.KnowledgeScopeHome{return filepath.Join(service.userRoot,"LYRA.md"),nil};resolved,err:=service.resolve(ctx,workspace);if err!=nil{return "",err};root:=resolved.Workspace.Path();if scope==protocol.KnowledgeScopeProjectRoot&&resolved.ProjectRoot!=""{root=resolved.ProjectRoot};return filepath.Join(root,"LYRA.md"),nil}
func (service *Service) memoryProject(ctx context.Context,scope protocol.AgentMemoryScope,workspace *protocol.WorkspaceRef)(string,error){if scope==protocol.AgentMemoryScopeUser{if workspace!=nil{return "",fmt.Errorf("%w: user memory forbids workspace",protocol.ErrInvalidParams)};return "",nil};if scope!=protocol.AgentMemoryScopeProject||workspace==nil{return "",fmt.Errorf("%w: project memory requires workspace",protocol.ErrInvalidParams)};resolved,err:=service.resolve(ctx,workspace);if err!=nil{return "",err};if resolved.ProjectRoot!=""{return resolved.ProjectRoot,nil};return resolved.Workspace.Path(),nil}

func rootToLeaf(root,leaf string)[]string{root=filepath.Clean(root);leaf=filepath.Clean(leaf);values:=[]string{};for current:=leaf;;current=filepath.Dir(current){values=append(values,current);if current==root||filepath.Dir(current)==current{break}};slices.Reverse(values);return values}
func revision(data []byte)string{sum:=sha256.Sum256(data);return hex.EncodeToString(sum[:])}
func readOptional(path string)([]byte,time.Time,error){info,err:=os.Stat(path);if errors.Is(err,os.ErrNotExist){return nil,time.Time{},nil};if err!=nil{return nil,time.Time{},err};if !info.Mode().IsRegular()||info.Size()>maxAuthoredDocumentBytes{return nil,time.Time{},fmt.Errorf("capabilityflow: invalid authored document %s",path)};data,err:=os.ReadFile(path);return data,info.ModTime(),err}
func atomicWrite(path string,data []byte,mode os.FileMode)error{if err:=os.MkdirAll(filepath.Dir(path),0o755);err!=nil{return err};temporary,err:=os.CreateTemp(filepath.Dir(path),".lyra-write-*");if err!=nil{return err};name:=temporary.Name();committed:=false;defer func(){_ = temporary.Close();if !committed{_ = os.Remove(name)}}();if err:=temporary.Chmod(mode);err!=nil{return err};if _,err:=temporary.Write(data);err!=nil{return err};if err:=temporary.Sync();err!=nil{return err};if err:=temporary.Close();err!=nil{return err};if err:=os.Rename(name,path);err!=nil{return err};committed=true;return nil}
