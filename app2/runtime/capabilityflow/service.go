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
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	lyraskills "github.com/Tangerg/lynx/skills"
	"gopkg.in/yaml.v3"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/sqlite"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const maxAuthoredDocumentBytes = 1 << 20

type Store interface {
	ListManagedSkillRecords(context.Context) ([]protocol.ManagedSkill, error)
	SetManagedSkillLifecycle(context.Context, string, protocol.SkillLifecycle) error
	PutManagedSkill(context.Context, protocol.ManagedSkill) error
	ListSkillProposalRecords(context.Context, string) ([]protocol.SkillProposal, error)
	GetSkillProposalRecord(context.Context, string, string, string) (protocol.SkillProposal, error)
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
	store Store
	resolver Resolver
	ids IDs
	home string
	userRoot string
	mu sync.Mutex
}

func New(store Store, resolver Resolver, ids IDs, home string) (*Service,error) {
	if store==nil||resolver==nil||ids==nil||!filepath.IsAbs(home){return nil,errors.New("capabilityflow: store, resolver, ids and absolute home are required")}
	return &Service{store:store,resolver:resolver,ids:ids,home:filepath.Clean(home),userRoot:filepath.Join(home,".lyra" )},nil
}

func (service *Service) DiscoveredSkills(ctx context.Context,query protocol.WorkspaceQuery)(*protocol.Page[protocol.Skill],error){
	resolved,err:=service.resolve(ctx,&query.Workspace);if err!=nil{return nil,err}
	managed,err:=service.ManagedSkills(ctx);if err!=nil{return nil,err}; archived:=map[string]bool{};for _,item:=range managed.Data{archived[item.Name]=item.Lifecycle==protocol.SkillLifecycleArchived}
	seen:=map[string]bool{}; values:=make([]protocol.Skill,0)
	add:=func(dir string,scope protocol.SkillScope)error{if !directoryExists(dir){return nil}; summaries,err:=lyraskills.Dir(dir).List(ctx);if err!=nil{return err};for _,summary:=range summaries{if seen[summary.Name]||archived[summary.Name]{continue};seen[summary.Name]=true;values=append(values,protocol.Skill{Name:summary.Name,Description:summary.Description,Scope:scope})};return nil}
	if err:=add(filepath.Join(resolved.Workspace.Path(),".lyra","skills"),protocol.SkillScopeProject);err!=nil{return nil,err}
	if err:=add(filepath.Join(service.userRoot,"skills"),protocol.SkillScopeUser);err!=nil{return nil,err}
	slices.SortFunc(values,func(a,b protocol.Skill)int{return strings.Compare(a.Name,b.Name)});return protocol.NewPage(values),nil
}

func (service *Service) ManagedSkills(ctx context.Context)(*protocol.Page[protocol.ManagedSkill],error){
	service.mu.Lock();defer service.mu.Unlock()
	stored,err:=service.store.ListManagedSkillRecords(ctx);if err!=nil{return nil,err};byName:=map[string]protocol.ManagedSkill{};for _,value:=range stored{byName[value.Name]=value}
	dir:=filepath.Join(service.userRoot,"skills");if directoryExists(dir){summaries,err:=lyraskills.Dir(dir).List(ctx);if err!=nil{return nil,err};for _,summary:=range summaries{if _,ok:=byName[summary.Name];ok{continue};value:=protocol.ManagedSkill{Name:summary.Name,Description:summary.Description,Lifecycle:protocol.SkillLifecycleActive};if err:=service.store.PutManagedSkill(ctx,value);err!=nil{return nil,err};byName[value.Name]=value}}
	values:=make([]protocol.ManagedSkill,0,len(byName));for _,value:=range byName{values=append(values,value)};slices.SortFunc(values,func(a,b protocol.ManagedSkill)int{return strings.Compare(a.Name,b.Name)});return protocol.NewPage(values),nil
}

func (service *Service) SetSkillLifecycle(ctx context.Context,request protocol.SkillNameRequest,lifecycle protocol.SkillLifecycle)error{if !validName(request.Name){return fmt.Errorf("%w: invalid skill name",protocol.ErrInvalidParams)};if err:=service.store.SetManagedSkillLifecycle(ctx,request.Name,lifecycle);err!=nil{if errors.Is(err,sqlite.ErrCapabilityNotFound){return protocol.ErrItemNotFound};return err};return nil}

func (service *Service) SkillProposals(ctx context.Context,query protocol.WorkspaceQuery)(*protocol.Page[protocol.SkillProposal],error){resolved,err:=service.resolve(ctx,&query.Workspace);if err!=nil{return nil,err};values,err:=service.store.ListSkillProposalRecords(ctx,resolved.Workspace.Path());if err!=nil{return nil,err};return protocol.NewPage(values),nil}

func (service *Service) ApproveProposal(ctx context.Context,ref protocol.SkillProposalRef)error{return service.settleProposal(ctx,ref,true)}
func (service *Service) RejectProposal(ctx context.Context,ref protocol.SkillProposalRef)error{return service.settleProposal(ctx,ref,false)}
func (service *Service) settleProposal(ctx context.Context,ref protocol.SkillProposalRef,approve bool)error{
	resolved,err:=service.resolve(ctx,&ref.Workspace);if err!=nil{return err}; proposal,err:=service.store.GetSkillProposalRecord(ctx,resolved.Workspace.Path(),ref.Name,ref.Revision);if err!=nil{return protocol.ErrItemNotFound}
	if proposal.Scope!=ref.Scope{return fmt.Errorf("%w: proposal scope mismatch",protocol.ErrInvalidParams)}
	if approve{root:=filepath.Join(service.userRoot,"skills");if proposal.Scope==protocol.SkillScopeProject{root=filepath.Join(resolved.Workspace.Path(),".lyra","skills")};dir:=filepath.Join(root,proposal.Name);content:="---\nname: "+proposal.Name+"\ndescription: "+yamlQuote(proposal.Description)+"\n---\n\n"+proposal.Instructions+"\n";if err:=atomicWrite(filepath.Join(dir,"SKILL.md"),[]byte(content),0o644);err!=nil{return err};if proposal.Scope==protocol.SkillScopeUser{_ = service.store.PutManagedSkill(ctx,protocol.ManagedSkill{Name:proposal.Name,Description:proposal.Description,Lifecycle:protocol.SkillLifecycleActive})}}
	return service.store.DeleteSkillProposalRecord(ctx,resolved.Workspace.Path(),ref.Name,ref.Revision)
}

func (service *Service) Recipes(ctx context.Context,query protocol.WorkspaceQuery)(*protocol.Page[protocol.Recipe],error){resolved,err:=service.resolve(ctx,&query.Workspace);if err!=nil{return nil,err};seen:=map[string]bool{};values:=make([]protocol.Recipe,0);add:=func(dir string,scope protocol.RecipeScope){entries,_:=os.ReadDir(dir);for _,entry:=range entries{if entry.IsDir()||strings.HasPrefix(entry.Name(),".")||!strings.HasSuffix(entry.Name(),".md"){continue};name:=strings.TrimSuffix(entry.Name(),".md");if seen[name]{continue};path:=filepath.Join(dir,entry.Name());data,err:=os.ReadFile(path);if err!=nil||len(data)>maxAuthoredDocumentBytes{continue};front,body:=parseRecipe(data);seen[name]=true;values=append(values,protocol.Recipe{Name:name,Description:front.Description,ArgumentHint:front.ArgumentHint,Body:body,Scope:scope,Source:path})}}
	add(filepath.Join(resolved.Workspace.Path(),".lyra","recipes"),protocol.RecipeScopeProject);add(filepath.Join(service.userRoot,"recipes"),protocol.RecipeScopeGlobal);slices.SortFunc(values,func(a,b protocol.Recipe)int{return strings.Compare(a.Name,b.Name)});return protocol.NewPage(values),nil}
}

func (service *Service) AgentDocs(ctx context.Context,query protocol.WorkspaceQuery)(*protocol.Page[protocol.AgentDoc],error){resolved,err:=service.resolve(ctx,&query.Workspace);if err!=nil{return nil,err};cwd:=resolved.Workspace.Path();root:=resolved.ProjectRoot;if root==""{root=cwd};seen:=map[string]bool{};values:=make([]protocol.AgentDoc,0);addFirst:=func(scope protocol.AgentDocScope,paths ...string){for _,path:=range paths{physical,err:=filepath.EvalSymlinks(path);if err!=nil||seen[physical]{continue};info,err:=os.Stat(physical);if err!=nil||!info.Mode().IsRegular()||info.Size()>maxAuthoredDocumentBytes{continue};seen[physical]=true;values=append(values,protocol.AgentDoc{Path:physical,Title:firstHeading(physical),Scope:scope});return}}
	addFirst(protocol.AgentDocScopeHome,filepath.Join(service.userRoot,"AGENTS.md"));addFirst(protocol.AgentDocScopeHome,filepath.Join(service.home,".agents","AGENTS.md"),filepath.Join(service.home,".agents","agents.md"));for _,dir:=range rootToLeaf(root,cwd){scope:=protocol.AgentDocScopeProjectRoot;if dir==cwd{scope=protocol.AgentDocScopeCWD};addFirst(scope,filepath.Join(dir,".lyra","AGENTS.md"));addFirst(scope,filepath.Join(dir,"AGENTS.md"),filepath.Join(dir,"agents.md"))};return protocol.NewPage(values),nil
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

type recipeFrontmatter struct{Description string `yaml:"description"`;ArgumentHint string `yaml:"argumentHint"`}
func parseRecipe(data []byte)(recipeFrontmatter,string){text:=strings.TrimPrefix(strings.ReplaceAll(string(data),"\r\n","\n"),"\ufeff");lines:=strings.Split(text,"\n");if len(lines)==0||strings.TrimSpace(lines[0])!="---"{return recipeFrontmatter{},strings.TrimSpace(text)};end:=-1;for i:=1;i<len(lines);i++{if strings.TrimSpace(lines[i])=="---"{end=i;break}};if end<0{return recipeFrontmatter{},strings.TrimSpace(text)};var front recipeFrontmatter;if yaml.Unmarshal([]byte(strings.Join(lines[1:end],"\n")),&front)!=nil{return recipeFrontmatter{},strings.TrimSpace(text)};front.Description=strings.TrimSpace(front.Description);front.ArgumentHint=strings.TrimSpace(front.ArgumentHint);return front,strings.TrimSpace(strings.Join(lines[end+1:],"\n"))}
var safeName=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
func validName(value string)bool{return safeName.MatchString(value)}
func directoryExists(path string)bool{info,err:=os.Stat(path);return err==nil&&info.IsDir()}
func firstHeading(path string)string{data,err:=os.ReadFile(path);if err!=nil{return ""};for _,line:=range strings.Split(string(data),"\n"){line=strings.TrimSpace(line);if strings.HasPrefix(line,"# "){return strings.TrimSpace(strings.TrimPrefix(line,"# "))}};return filepath.Base(path)}
func rootToLeaf(root,leaf string)[]string{root=filepath.Clean(root);leaf=filepath.Clean(leaf);values:=[]string{};for current:=leaf;;current=filepath.Dir(current){values=append(values,current);if current==root||filepath.Dir(current)==current{break}};slices.Reverse(values);return values}
func revision(data []byte)string{sum:=sha256.Sum256(data);return hex.EncodeToString(sum[:])}
func readOptional(path string)([]byte,time.Time,error){info,err:=os.Stat(path);if errors.Is(err,os.ErrNotExist){return nil,time.Time{},nil};if err!=nil{return nil,time.Time{},err};if !info.Mode().IsRegular()||info.Size()>maxAuthoredDocumentBytes{return nil,time.Time{},fmt.Errorf("capabilityflow: invalid authored document %s",path)};data,err:=os.ReadFile(path);return data,info.ModTime(),err}
func atomicWrite(path string,data []byte,mode os.FileMode)error{if err:=os.MkdirAll(filepath.Dir(path),0o755);err!=nil{return err};temporary,err:=os.CreateTemp(filepath.Dir(path),".lyra-write-*");if err!=nil{return err};name:=temporary.Name();committed:=false;defer func(){_ = temporary.Close();if !committed{_ = os.Remove(name)}}();if err:=temporary.Chmod(mode);err!=nil{return err};if _,err:=temporary.Write(data);err!=nil{return err};if err:=temporary.Sync();err!=nil{return err};if err:=temporary.Close();err!=nil{return err};if err:=os.Rename(name,path);err!=nil{return err};committed=true;return nil}
func yamlQuote(value string)string{encoded,_:=yaml.Marshal(value);return strings.TrimSpace(string(encoded))}
