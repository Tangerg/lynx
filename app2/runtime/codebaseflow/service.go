// Package codebaseflow owns the asynchronous semantic source index.
package codebaseflow

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/app2/runtime/domain/codebase"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const (
	maxIndexFiles = 5000
	maxIndexBytes = 64 << 20
	chunkLines = 80
	embedBatch = 32
)

type Store interface {
	GetCodebaseIndex(context.Context, string) (codebase.Index, error)
	PutCodebaseIndexState(context.Context, codebase.Index) error
	ReplaceCodebaseDocuments(context.Context, codebase.Index, []codebase.Document) error
	ListCodebaseDocuments(context.Context, string) ([]codebase.Document, error)
}
type Resolver interface { Resolve(context.Context, string) (workspacefs.Resolution, error) }
type Models interface { ResolveEmbedding(context.Context) (embedding.Model, protocol.EmbeddingRole, error) }
type IDs interface { New(string) (string, error) }
type Publisher interface { Publish(protocol.RuntimeEvent) }

type activeTask struct { generation uint64; cancel context.CancelFunc }
type Service struct {
	store Store
	resolver Resolver
	models Models
	ids IDs
	events Publisher
	lifetime context.Context
	mu sync.Mutex
	nextGeneration uint64
	active map[string]activeTask
	tasks sync.WaitGroup
	closed bool
}

func New(store Store, resolver Resolver, models Models, ids IDs, events Publisher, lifetime context.Context) (*Service,error) {
	if store==nil||resolver==nil||models==nil||ids==nil||events==nil||lifetime==nil{return nil,errors.New("codebaseflow: dependencies are required")}
	return &Service{store:store,resolver:resolver,models:models,ids:ids,events:events,lifetime:lifetime,active:map[string]activeTask{}},nil
}

func(s *Service) Status(ctx context.Context, request protocol.CodebaseStatusRequest)(*protocol.CodebaseStatus,error){
	workspace,err:=s.workspace(ctx,request.Workspace);if err!=nil{return nil,err}
	index,err:=s.store.GetCodebaseIndex(ctx,workspace);if err!=nil{return nil,err}
	return present(index),nil
}

func(s *Service) Reindex(ctx context.Context, request protocol.CodebaseReindexRequest)(*protocol.CodebaseReindexResponse,error){
	workspace,err:=s.workspace(ctx,request.Workspace);if err!=nil{return nil,err}
	operationID,err:=s.ids.New("idx_");if err!=nil{return nil,err}
	model,role,err:=s.models.ResolveEmbedding(ctx);if err!=nil{return nil,err}
	s.mu.Lock()
	if s.closed{s.mu.Unlock();return nil,errors.New("codebaseflow: closed")}
	if current,ok:=s.active[workspace];ok{current.cancel()}
	s.nextGeneration++;generation:=s.nextGeneration
	taskCtx,cancel:=context.WithCancel(s.lifetime);s.active[workspace]=activeTask{generation:generation,cancel:cancel};s.tasks.Add(1);s.mu.Unlock()
	index:=codebase.Index{Workspace:workspace,State:string(protocol.CodebaseStateIndexing),OperationID:operationID,ModelID:role.Provider+"/"+role.Model}
	if err:=s.store.PutCodebaseIndexState(ctx,index);err!=nil{cancel();s.finish(workspace,generation);return nil,err}
	s.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeCodebaseChanged})
	go s.build(taskCtx,index,model,generation)
	return &protocol.CodebaseReindexResponse{OperationID:operationID},nil
}

func(s *Service) Search(ctx context.Context, request protocol.CodebaseSearchRequest)(*protocol.CodebaseSearchResult,error){
	if strings.TrimSpace(request.Query)==""{return nil,fmt.Errorf("%w: query is required",protocol.ErrInvalidParams)}
	workspace,err:=s.workspace(ctx,request.Workspace);if err!=nil{return nil,err}
	index,err:=s.store.GetCodebaseIndex(ctx,workspace);if err!=nil{return nil,err}
	if index.State!=string(protocol.CodebaseStateReady){return &protocol.CodebaseSearchResult{Hits:[]protocol.CodebaseHit{}},nil}
	model,_,err:=s.models.ResolveEmbedding(ctx);if err!=nil{return nil,err}
	vectors,err:=embed(ctx,model,[]string{request.Query});if err!=nil{return nil,err}
	documents,err:=s.store.ListCodebaseDocuments(ctx,workspace);if err!=nil{return nil,err}
	hits:=make([]protocol.CodebaseHit,0,len(documents))
	for _,document:=range documents{score:=cosine(vectors[0],document.Vector);if score<=0{continue};hits=append(hits,protocol.CodebaseHit{Path:document.Path,StartLine:document.StartLine,EndLine:document.EndLine,Snippet:document.Snippet,Score:score})}
	slices.SortFunc(hits,func(a,b protocol.CodebaseHit)int{if a.Score>b.Score{return -1};if a.Score<b.Score{return 1};return strings.Compare(a.Path,b.Path)})
	limit:=request.Limit;if limit<=0{limit=8};limit=min(limit,50);if len(hits)>limit{hits=hits[:limit]}
	return &protocol.CodebaseSearchResult{Hits:hits},nil
}

func(s *Service) Close(){s.mu.Lock();if s.closed{s.mu.Unlock();return};s.closed=true;for _,task:=range s.active{task.cancel()};s.mu.Unlock();s.tasks.Wait()}

func(s *Service) build(ctx context.Context,index codebase.Index,model embedding.Model,generation uint64){
	defer s.tasks.Done();defer s.finish(index.Workspace,generation)
	documents,files,truncated,err:=scan(ctx,index.Workspace)
	if err==nil{for begin:=0;begin<len(documents);begin+=embedBatch{end:=min(begin+embedBatch,len(documents));texts:=make([]string,end-begin);for i:=begin;i<end;i++{texts[i-begin]=documents[i].Snippet};vectors,embedErr:=embed(ctx,model,texts);if embedErr!=nil{err=embedErr;break};for i:=range vectors{documents[begin+i].Vector=vectors[i]}}}
	if ctx.Err()!=nil{return}
	persistCtx,cancel:=context.WithTimeout(context.WithoutCancel(ctx),10*time.Second);defer cancel()
	now:=time.Now().UTC()
	if err!=nil{index.State=string(protocol.CodebaseStateError);_ = s.store.PutCodebaseIndexState(persistCtx,index)}else{index.State=string(protocol.CodebaseStateReady);index.FileCount=files;index.ChunkCount=len(documents);index.Truncated=truncated;index.IndexedAt=&now;_ = s.store.ReplaceCodebaseDocuments(persistCtx,index,documents)}
	s.events.Publish(protocol.RuntimeEvent{Type:protocol.RuntimeCodebaseChanged})
}

func(s *Service) finish(workspace string,generation uint64){s.mu.Lock();if current,ok:=s.active[workspace];ok&&current.generation==generation{current.cancel();delete(s.active,workspace)};s.mu.Unlock()}
func(s *Service) workspace(ctx context.Context,ref protocol.WorkspaceRef)(string,error){resolved,err:=s.resolver.Resolve(ctx,ref.Path);if err!=nil||!resolved.Available{return "",protocol.ErrWorkspaceUnavailable};if resolved.ProjectRoot!=""{return resolved.ProjectRoot,nil};return resolved.Workspace.Path(),nil}

func scan(ctx context.Context,root string)([]codebase.Document,int,bool,error){
	documents:=make([]codebase.Document,0);files:=0;var total int64;truncated:=false
	err:=filepath.WalkDir(root,func(path string,entry os.DirEntry,walkErr error)error{
		if walkErr!=nil{return nil};if err:=ctx.Err();err!=nil{return err}
		if entry.IsDir(){if path!=root&&(entry.Name()==".git"||entry.Name()=="node_modules"||entry.Name()=="vendor"||entry.Name()=="dist"||entry.Name()=="build"){return filepath.SkipDir};return nil}
		info,err:=entry.Info();if err!=nil||info.Size()>2<<20{return nil};if files>=maxIndexFiles||total+info.Size()>maxIndexBytes{truncated=true;return nil}
		file,err:=os.Open(path);if err!=nil{return nil};scanner:=bufio.NewScanner(file);scanner.Buffer(make([]byte,64<<10),2<<20);lines:=make([]string,0)
		for scanner.Scan(){line:=scanner.Text();if strings.IndexByte(line,0)>=0{lines=nil;break};lines=append(lines,line)}
		closeErr:=file.Close();if scanner.Err()!=nil||closeErr!=nil||len(lines)==0{return nil}
		relative,_:=filepath.Rel(root,path);for start:=0;start<len(lines);start+=chunkLines{end:=min(start+chunkLines,len(lines));snippet:=strings.Join(lines[start:end],"\n");if strings.TrimSpace(snippet)==""{continue};documents=append(documents,codebase.Document{Path:filepath.ToSlash(relative),StartLine:start+1,EndLine:end,Snippet:snippet})}
		files++;total+=info.Size();return nil
	})
	return documents,files,truncated,err
}

func embed(ctx context.Context,model embedding.Model,texts []string)([][]float64,error){request,err:=embedding.NewRequest(texts);if err!=nil{return nil,err};response,err:=model.Call(ctx,request);if err!=nil{return nil,err};vectors:=make([][]float64,len(response.Results));for i,result:=range response.Results{vectors[i]=slices.Clone(result.Embedding)};return vectors,nil}
func cosine(left,right []float64)float64{if len(left)==0||len(left)!=len(right){return 0};var dot,a,b float64;for i:=range left{dot+=left[i]*right[i];a+=left[i]*left[i];b+=right[i]*right[i]};if a==0||b==0{return 0};score:=dot/(math.Sqrt(a)*math.Sqrt(b));return max(0,min(1,score))}
func present(index codebase.Index)*protocol.CodebaseStatus{value:=&protocol.CodebaseStatus{State:protocol.CodebaseState(index.State),ModelID:index.ModelID,FileCount:index.FileCount,ChunkCount:index.ChunkCount,Truncated:index.Truncated,OperationID:index.OperationID};if index.IndexedAt!=nil{value.IndexedAt=index.IndexedAt.Format(time.RFC3339)};return value}
