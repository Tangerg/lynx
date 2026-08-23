// Package codeintel owns lazy language-server processes for one Runtime
// instance. It confines file queries to the mounted workspace and exposes a
// provider-neutral query port to the tool catalog.
package codeintel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

var (
	ErrUnsupported = errors.New("codeintel: no configured language server supports the target")
	ErrUnavailable = errors.New("codeintel: configured language server is unavailable")
	ErrClosed = errors.New("codeintel: service is closed")
)

const maxDocumentBytes = 8 << 20

type Service struct {
	ctx context.Context
	cancel context.CancelFunc
	specs []ServerSpec
	byExtension map[string]ServerSpec

	mu sync.Mutex
	clients map[string]*client
	starting map[string]*launch
	closed bool
	watchers sync.WaitGroup
	once sync.Once
	closeErr error
}

type client struct {
	spec ServerSpec
	root string
	command *exec.Cmd
	connection *jsonrpc2.Conn
	cancel context.CancelFunc
	exited <-chan struct{}

	mu sync.Mutex
	waitErr error
	open map[string]openDocument
	diagnostics map[string][]diagnostic
	updates map[string]chan struct{}
	closeOnce sync.Once
	closeErr error
}

type openDocument struct { Version int; Digest [32]byte }

type launch struct {
	done chan struct{}
	cancel context.CancelFunc
	client *client
	err error
}

func New(lifetime context.Context, specs []ServerSpec) (*Service, error) {
	if lifetime == nil { return nil, errors.New("codeintel: lifetime is required") }
	if len(specs) == 0 { specs = DefaultServers() }
	byExtension := make(map[string]ServerSpec)
	seen := make(map[string]struct{}, len(specs))
	owned := make([]ServerSpec, len(specs))
	for index, spec := range specs {
		if strings.TrimSpace(spec.Name) != spec.Name || strings.TrimSpace(spec.Command) != spec.Command || strings.TrimSpace(spec.LanguageID) != spec.LanguageID || spec.Name == "" || spec.Command == "" || spec.LanguageID == "" { return nil, errors.New("codeintel: every server needs a trimmed name, command, and language id") }
		if len(spec.Extensions) == 0 { return nil, fmt.Errorf("codeintel: server %q needs at least one extension", spec.Name) }
		if _, duplicate := seen[spec.Name]; duplicate { return nil, fmt.Errorf("codeintel: duplicate server %q", spec.Name) }
		seen[spec.Name] = struct{}{}
		spec.Args, spec.Extensions, spec.RootMarkers = slices.Clone(spec.Args), slices.Clone(spec.Extensions), slices.Clone(spec.RootMarkers)
		for extensionIndex, extension := range spec.Extensions {
			extension = strings.ToLower(strings.TrimSpace(extension))
			if len(extension) < 2 || !strings.HasPrefix(extension, ".") || strings.ContainsAny(extension, `/\\`) { return nil, fmt.Errorf("codeintel: invalid extension %q", extension) }
			if _, duplicate := byExtension[extension]; duplicate { return nil, fmt.Errorf("codeintel: extension %q has multiple owners", extension) }
			spec.Extensions[extensionIndex] = extension
			byExtension[extension] = spec
		}
		for markerIndex, marker := range spec.RootMarkers {
			marker = filepath.Clean(strings.TrimSpace(marker))
			if marker == "." || filepath.IsAbs(marker) || marker == ".." || strings.HasPrefix(marker, ".."+string(filepath.Separator)) { return nil, fmt.Errorf("codeintel: invalid root marker %q", marker) }
			spec.RootMarkers[markerIndex] = marker
		}
		owned[index] = spec
	}
	ctx, cancel := context.WithCancel(lifetime)
	return &Service{ctx:ctx,cancel:cancel,specs:owned,byExtension:byExtension,clients:make(map[string]*client),starting:make(map[string]*launch)}, nil
}

func (service *Service) clientFor(ctx context.Context, root, path string) (*client, error) {
	spec, found := service.byExtension[strings.ToLower(filepath.Ext(path))]
	if !found { return nil, fmt.Errorf("%w: %s", ErrUnsupported, filepath.Ext(path)) }
	return service.server(ctx, root, spec)
}

func (service *Service) server(ctx context.Context, root string, spec ServerSpec) (*client, error) {
	key := root+"\x00"+spec.Name
	service.mu.Lock()
	if service.closed { service.mu.Unlock();return nil, ErrClosed }
	if value := service.clients[key]; value != nil { service.mu.Unlock();return value, nil }
	if pending:=service.starting[key];pending!=nil{service.mu.Unlock();select{case<-pending.done:return pending.client,pending.err;case<-ctx.Done():return nil,ctx.Err()}}
	if err := ctx.Err(); err != nil { service.mu.Unlock();return nil, err }
	launchContext,cancel:=context.WithCancel(service.ctx);pending:=&launch{done:make(chan struct{}),cancel:cancel};service.starting[key]=pending;service.mu.Unlock()
	go service.launchClient(launchContext,key,root,spec,pending)
	select{case<-pending.done:return pending.client,pending.err;case<-ctx.Done():return nil,ctx.Err()}
}

func(service *Service)launchClient(launchContext context.Context,key,root string,spec ServerSpec,pending *launch){
	value,err:=startClient(launchContext,root,spec);pending.cancel()
	service.mu.Lock();closed:=service.closed
	if !closed{delete(service.starting,key);if err==nil{service.clients[key]=value;pending.client=value;service.watchers.Add(1);go service.watchClient(key,value)}else{pending.err=err};close(pending.done);service.mu.Unlock();return};service.mu.Unlock()
	if value!=nil{_=value.close()}
	service.mu.Lock();delete(service.starting,key);if err==nil{pending.err=ErrClosed}else{pending.err=err};close(pending.done);service.mu.Unlock()
}

func (service *Service) watchClient(key string, value *client) {
	defer service.watchers.Done()
	<-value.exited
	value.cancel()
	service.mu.Lock()
	if service.clients[key] == value {
		delete(service.clients, key)
	}
	service.mu.Unlock()
}

func startClient(lifetime context.Context, root string, spec ServerSpec) (*client, error) {
	command := exec.Command(spec.Command, spec.Args...)
	command.Dir = root
	command.Stderr = io.Discard
	configureCommand(command)
	stdout, err := command.StdoutPipe()
	if err != nil { return nil, fmt.Errorf("codeintel: open %s stdout: %w", spec.Name, err) }
	stdin, err := command.StdinPipe()
	if err != nil { _=stdout.Close(); return nil, fmt.Errorf("codeintel: open %s stdin: %w", spec.Name, err) }
	if err := command.Start(); err != nil { _=stdout.Close(); _=stdin.Close(); if errors.Is(err,exec.ErrNotFound){return nil,fmt.Errorf("%w: %s",ErrUnavailable,spec.Name)};return nil, fmt.Errorf("codeintel: start %s: %w", spec.Name, err) }
	exited := make(chan struct{})
	connectionContext, cancel := context.WithCancel(context.WithoutCancel(lifetime))
	value := &client{spec:spec,root:root,command:command,cancel:cancel,exited:exited,open:make(map[string]openDocument),diagnostics:make(map[string][]diagnostic),updates:make(map[string]chan struct{})}
	go func(){ err:=command.Wait();value.mu.Lock();value.waitErr=err;value.mu.Unlock();close(exited) }()
	stream := jsonrpc2.NewBufferedStream(&processStream{Reader:stdout,Writer:stdin}, jsonrpc2.VSCodeObjectCodec{})
	value.connection = jsonrpc2.NewConn(connectionContext, stream, jsonrpc2.AsyncHandler(value))
	initializeContext, stop := context.WithTimeout(lifetime, 30*time.Second)
	defer stop()
	params := map[string]any{
		"processId":os.Getpid(), "rootUri":pathURI(root),
		"workspaceFolders":[]map[string]string{{"uri":pathURI(root),"name":filepath.Base(root)}},
		"capabilities":map[string]any{
			"textDocument":map[string]any{"synchronization":map[string]any{"dynamicRegistration":false},"definition":map[string]any{},"references":map[string]any{},"implementation":map[string]any{},"hover":map[string]any{"contentFormat":[]string{"markdown","plaintext"}},"documentSymbol":map[string]any{"hierarchicalDocumentSymbolSupport":true},"callHierarchy":map[string]any{},"publishDiagnostics":map[string]any{}},
			"workspace":map[string]any{"symbol":map[string]any{},"configuration":true,"workspaceFolders":true},
		},
	}
	var initialized json.RawMessage
	if err := value.connection.Call(initializeContext,"initialize",params,&initialized); err != nil { _=value.close(); return nil, fmt.Errorf("codeintel: initialize %s: %w",spec.Name,err) }
	if err := value.connection.Notify(initializeContext,"initialized",struct{}{}); err != nil { _=value.close(); return nil, fmt.Errorf("codeintel: acknowledge %s initialization: %w",spec.Name,err) }
	return value,nil
}

type processStream struct { io.Reader; io.Writer }
func (stream *processStream) Close() error {
	var errs []error
	if closer,ok:=stream.Writer.(io.Closer); ok { errs=append(errs,closer.Close()) }
	if closer,ok:=stream.Reader.(io.Closer); ok { errs=append(errs,closer.Close()) }
	return errors.Join(errs...)
}

func (value *client) Handle(ctx context.Context, connection *jsonrpc2.Conn, request *jsonrpc2.Request) {
	switch request.Method {
	case "textDocument/publishDiagnostics":
		if request.Params==nil { return }
		var params struct { URI string `json:"uri"`; Diagnostics []diagnostic `json:"diagnostics"` }
		if json.Unmarshal(*request.Params,&params)!=nil || params.URI=="" { return }
		value.mu.Lock()
		value.diagnostics[params.URI]=slices.Clone(params.Diagnostics)
		if signal:=value.updates[params.URI]; signal!=nil { close(signal) }
		value.updates[params.URI]=make(chan struct{})
		value.mu.Unlock()
	case "workspace/configuration":
		if !request.Notif { var params struct{Items []json.RawMessage `json:"items"`};if request.Params!=nil{_=json.Unmarshal(*request.Params,&params)};_ = connection.Reply(ctx,request.ID,make([]any,len(params.Items))) }
	default:
		if !request.Notif { _=connection.Reply(ctx,request.ID,nil) }
	}
}

func (value *client) sync(ctx context.Context, path string) (string,<-chan struct{},bool,error) {
	contents,err:=readDocument(path)
	if err!=nil { return "",nil,false,fmt.Errorf("codeintel: read %s: %w",path,err) }
	uri:=pathURI(path); digest:=sha256.Sum256(contents)
	value.mu.Lock(); defer value.mu.Unlock()
	previous,opened:=value.open[uri]
	signal:=value.updates[uri];if signal==nil{signal=make(chan struct{});value.updates[uri]=signal}
	if opened && previous.Digest==digest { return uri,signal,false,nil }
	version:=previous.Version+1
	var notifyErr error
	if !opened {
		notifyErr=value.connection.Notify(ctx,"textDocument/didOpen",map[string]any{"textDocument":map[string]any{"uri":uri,"languageId":value.spec.LanguageID,"version":version,"text":string(contents)}})
	} else {
		notifyErr=value.connection.Notify(ctx,"textDocument/didChange",map[string]any{"textDocument":map[string]any{"uri":uri,"version":version},"contentChanges":[]map[string]string{{"text":string(contents)}}})
	}
	if notifyErr!=nil { return "",nil,false,fmt.Errorf("codeintel: synchronize %s: %w",path,notifyErr) }
	value.open[uri]=openDocument{Version:version,Digest:digest}
	return uri,signal,true,nil
}

func readDocument(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil { return nil, err }
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil { return nil, err }
	if len(contents) > maxDocumentBytes { return nil, errors.New("language-server document exceeds the 8 MiB limit") }
	return contents, nil
}

func (value *client) close() error {
	value.closeOnce.Do(func(){
		ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second); defer cancel()
		_=value.connection.Call(ctx,"shutdown",nil,nil); _=value.connection.Notify(ctx,"exit",nil)
		value.cancel(); _=value.connection.Close()
		select { case <-value.exited: value.mu.Lock();value.closeErr=value.waitErr;value.mu.Unlock();case <-ctx.Done(): _=killCommand(value.command); <-value.exited }
	})
	return value.closeErr
}

func (service *Service) Close() error {
	service.once.Do(func(){
		service.mu.Lock(); service.closed=true; clients:=service.clients; service.clients=make(map[string]*client);starting:=make([]*launch,0,len(service.starting));for _,pending:=range service.starting{starting=append(starting,pending)};service.mu.Unlock()
		for _,pending:=range starting{pending.cancel()}
		keys:=make([]string,0,len(clients)); for key:=range clients { keys=append(keys,key) }; slices.Sort(keys)
		var errs []error; for _,key:=range keys { if err:=clients[key].close(); err!=nil { errs=append(errs,err) } };for _,pending:=range starting{<-pending.done};service.watchers.Wait();service.cancel();service.closeErr=errors.Join(errs...)
	})
	return service.closeErr
}
