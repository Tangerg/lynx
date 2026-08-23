// Package remote owns Desktop attachment to a user-configured remote Runtime.
// The endpoint identity is persisted without credentials; the bearer secret is
// held by the operating-system keyring and enters the renderer only in the
// active connection bootstrap.
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/Tangerg/lynx/app2/desktop/supervisor"
	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	keyringService = "io.lyra.desktop.remote-runtime"
	keyringUser = "active"
	maxSecretBytes = 4096
)

type Profile struct {
	Endpoint string `json:"endpoint"`
	ServerName string `json:"serverName"`
	Active bool `json:"active"`
}

type State struct {
	Configured bool `json:"configured"`
	Active bool `json:"active"`
	Endpoint string `json:"endpoint,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	Connected bool `json:"connected"`
	Detail string `json:"detail,omitempty"`
}

type secrets interface { Set(string,string,string)error; Get(string,string)(string,error); Delete(string,string)error }
type systemSecrets struct{}
func(systemSecrets)Set(service,user,value string)error{return keyring.Set(service,user,value)}
func(systemSecrets)Get(service,user string)(string,error){return keyring.Get(service,user)}
func(systemSecrets)Delete(service,user string)error{return keyring.Delete(service,user)}

type Manager struct {
	path string
	secrets secrets
	client *http.Client
	operations sync.Mutex
	mu sync.Mutex
	profile *Profile
	instanceID string
	generation uint64
	connected bool
	detail string
}

func(manager *Manager)Close(){if manager==nil{return};manager.operations.Lock();defer manager.operations.Unlock();manager.client.CloseIdleConnections();manager.mu.Lock();manager.connected=false;manager.mu.Unlock()}

func Open(path string)(*Manager,error){return open(path,systemSecrets{})}
func open(path string,store secrets)(*Manager,error){
	if !filepath.IsAbs(path)||store==nil{return nil,errors.New("remote: absolute profile path and secret store are required")}
	transport:=&http.Transport{Proxy:http.ProxyFromEnvironment,ResponseHeaderTimeout:10*time.Second,IdleConnTimeout:90*time.Second}
	manager:=&Manager{path:filepath.Clean(path),secrets:store,client:&http.Client{Transport:transport,Timeout:30*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}}
	profile,err:=readProfile(manager.path);if err!=nil&&!errors.Is(err,os.ErrNotExist){return nil,err};manager.profile=profile;return manager,nil
}

func(manager *Manager)Active()bool{manager.mu.Lock();defer manager.mu.Unlock();return manager.profile!=nil&&manager.profile.Active}

func(manager *Manager)State()State{manager.mu.Lock();defer manager.mu.Unlock();return stateOf(manager.profile,manager.connected,manager.detail)}

func(manager *Manager)Bootstrap(ctx context.Context)(supervisor.Connection,error){
	manager.operations.Lock();defer manager.operations.Unlock()
	manager.mu.Lock();if manager.profile==nil||!manager.profile.Active{manager.mu.Unlock();return supervisor.Connection{},errors.New("remote: no active remote Runtime")};profile:=*manager.profile;manager.mu.Unlock()
	secret,err:=manager.secrets.Get(keyringService,keyringUser);if err!=nil{err=fmt.Errorf("remote: read bearer secret: %w",err);manager.recordFailure(err);return supervisor.Connection{},err}
	connection,serverName,err:=manager.probe(ctx,profile.Endpoint,secret);if err!=nil{manager.recordFailure(err);return supervisor.Connection{},err};if serverName!=profile.ServerName{err=errors.New("remote: Runtime server identity changed");manager.recordFailure(err);return supervisor.Connection{},err}
	manager.mu.Lock();if manager.instanceID!=connection.InstanceID{manager.instanceID=connection.InstanceID;manager.generation++;if manager.generation==0{manager.generation=1}};connection.Generation=manager.generation;manager.connected=true;manager.detail="";manager.mu.Unlock();return connection,nil
}

func(manager *Manager)Configure(ctx context.Context,endpoint,secret string)(State,error){
	manager.operations.Lock();defer manager.operations.Unlock()
	endpoint,err:=normalizeEndpoint(endpoint);if err!=nil{return State{},err};if strings.TrimSpace(secret)!=secret||len(secret)<16||len(secret)>maxSecretBytes{return State{},errors.New("remote: a bounded trimmed bearer secret is required")}
	connection,serverName,err:=manager.probe(ctx,endpoint,secret);if err!=nil{return State{},err};profile:=&Profile{Endpoint:endpoint,ServerName:serverName,Active:true}
	previousSecret,previousSecretErr:=manager.secrets.Get(keyringService,keyringUser);if err:=manager.secrets.Set(keyringService,keyringUser,secret);err!=nil{return State{},fmt.Errorf("remote: store bearer secret: %w",err)}
	if err:=writeProfile(manager.path,*profile);err!=nil{if previousSecretErr==nil{_=manager.secrets.Set(keyringService,keyringUser,previousSecret)}else{_=manager.secrets.Delete(keyringService,keyringUser)};return State{},err}
	manager.mu.Lock();manager.profile=profile;manager.instanceID=connection.InstanceID;manager.generation++;if manager.generation==0{manager.generation=1};manager.connected=true;manager.detail="";manager.mu.Unlock();return stateOf(profile,true,""),nil
}

func(manager *Manager)UseLocal()error{manager.operations.Lock();defer manager.operations.Unlock();manager.mu.Lock();defer manager.mu.Unlock();if manager.profile==nil{return nil};next:=*manager.profile;next.Active=false;if err:=writeProfile(manager.path,next);err!=nil{return err};manager.profile=&next;manager.connected=false;manager.detail="";return nil}

func(manager *Manager)UseRemote(ctx context.Context)(State,error){
	manager.operations.Lock();defer manager.operations.Unlock();manager.mu.Lock();if manager.profile==nil{manager.mu.Unlock();return State{},errors.New("remote: no configured remote Runtime")};profile:=*manager.profile;manager.mu.Unlock()
	secret,err:=manager.secrets.Get(keyringService,keyringUser);if err!=nil{return State{},fmt.Errorf("remote: read bearer secret: %w",err)};connection,serverName,err:=manager.probe(ctx,profile.Endpoint,secret);if err!=nil{manager.recordFailure(err);return State{},err};if serverName!=profile.ServerName{err=errors.New("remote: Runtime server identity changed");manager.recordFailure(err);return State{},err}
	profile.Active=true;if err:=writeProfile(manager.path,profile);err!=nil{return State{},err};manager.mu.Lock();manager.profile=&profile;manager.instanceID=connection.InstanceID;manager.generation++;if manager.generation==0{manager.generation=1};manager.connected=true;manager.detail="";manager.mu.Unlock();return stateOf(&profile,true,""),nil
}

func(manager *Manager)Forget()error{
	manager.operations.Lock();defer manager.operations.Unlock()
	manager.mu.Lock();var previous *Profile;if manager.profile!=nil{snapshot:=*manager.profile;previous=&snapshot};manager.mu.Unlock()
	if err:=os.Remove(manager.path);err!=nil&&!errors.Is(err,os.ErrNotExist){return fmt.Errorf("remote: remove profile: %w",err)}
	if err:=manager.secrets.Delete(keyringService,keyringUser);err!=nil&&!errors.Is(err,keyring.ErrNotFound){restoreErr:=error(nil);if previous!=nil{restoreErr=writeProfile(manager.path,*previous)};return errors.Join(fmt.Errorf("remote: remove bearer secret: %w",err),restoreErr)}
	manager.mu.Lock();manager.profile=nil;manager.instanceID="";manager.generation=0;manager.connected=false;manager.detail="";manager.mu.Unlock();return nil
}

func(manager *Manager)recordFailure(err error){manager.mu.Lock();manager.connected=false;manager.detail=err.Error();manager.mu.Unlock()}

func(manager *Manager)probe(ctx context.Context,endpoint,secret string)(supervisor.Connection,string,error){
	var info httptransport.RuntimeInfo;if err:=manager.getJSON(ctx,endpoint+httptransport.PathInfo,"",nil,&info);err!=nil{return supervisor.Connection{},"",fmt.Errorf("remote: inspect Runtime: %w",err)}
	var live httptransport.LivenessStatus;if err:=manager.getJSON(ctx,endpoint+httptransport.PathLiveness,"",nil,&live);err!=nil{return supervisor.Connection{},"",err}
	var ready httptransport.ReadinessStatus;if err:=manager.getJSON(ctx,endpoint+httptransport.PathReadiness,"",nil,&ready);err!=nil{return supervisor.Connection{},"",err}
	body:=[]byte(`{"jsonrpc":"2.0","id":"desktop-remote-bootstrap","method":"runtime.discover","params":{"_meta":{"protocolVersion":"`+protocol.ProtocolVersion+`"}}}`)
	var rpc struct{JSONRPC string `json:"jsonrpc"`;ID string `json:"id"`;Result protocol.DiscoverResponse `json:"result"`;Error json.RawMessage `json:"error,omitempty"`};if err:=manager.getJSON(ctx,endpoint+httptransport.PathRPC,secret,body,&rpc);err!=nil{return supervisor.Connection{},"",err};if len(rpc.Error)>0{return supervisor.Connection{},"",errors.New("remote: runtime.discover rejected the attachment")};if err:=rpc.Result.Validate();err!=nil{return supervisor.Connection{},"",err}
	instance:=info.Server.InstanceID;if instance==""||strings.TrimSpace(info.Server.Name)!=info.Server.Name||len(info.Server.Name)>256||rpc.JSONRPC!="2.0"||rpc.ID!="desktop-remote-bootstrap"||live.InstanceID!=instance||ready.InstanceID!=instance||rpc.Result.ServerInfo.InstanceID!=instance||rpc.Result.ServerInfo.Name!=info.Server.Name||rpc.Result.ServerInfo.Version!=info.Server.Version||info.ProtocolVersion!=protocol.ProtocolVersion||rpc.Result.ProtocolVersion!=protocol.ProtocolVersion||info.Transport!=httptransport.TransportHTTP||info.Endpoints.RPC!=httptransport.PathRPC||info.Endpoints.Info!=httptransport.PathInfo||info.Endpoints.Liveness!=httptransport.PathLiveness||info.Endpoints.Readiness!=httptransport.PathReadiness||live.Status!=httptransport.HealthOK||ready.Status!=httptransport.HealthOK{return supervisor.Connection{},"",errors.New("remote: Runtime identity, protocol, endpoints, or readiness did not agree")}
	return supervisor.Connection{Endpoint:endpoint,BearerToken:secret,InstanceID:instance,ProtocolVersion:protocol.ProtocolVersion,IdempotencyNamespace:rpc.Result.Capabilities.Limits.Idempotency.Namespace,Generation:1},info.Server.Name,nil
}

func(manager *Manager)getJSON(ctx context.Context,endpoint,secret string,body []byte,target any)error{method:=http.MethodGet;reader:=bytes.NewReader(nil);if body!=nil{method=http.MethodPost;reader=bytes.NewReader(body)};request,err:=http.NewRequestWithContext(ctx,method,endpoint,reader);if err!=nil{return err};request.Header.Set("Accept","application/json");if body!=nil{request.Header.Set("Content-Type","application/json");request.Header.Set("Authorization","Bearer "+secret)};response,err:=manager.client.Do(request);if err!=nil{return err};defer response.Body.Close();if response.StatusCode!=http.StatusOK{_,_=io.Copy(io.Discard,io.LimitReader(response.Body,4096));return fmt.Errorf("HTTP status %d",response.StatusCode)};mediaType,_,err:=mime.ParseMediaType(response.Header.Get("Content-Type"));if err!=nil||mediaType!="application/json"{return errors.New("remote: response is not application/json")};decoder:=json.NewDecoder(io.LimitReader(response.Body,4<<20));decoder.DisallowUnknownFields();if err:=decoder.Decode(target);err!=nil{return err};if err:=decoder.Decode(&struct{}{});!errors.Is(err,io.EOF){return errors.New("remote: response contains trailing JSON")};return nil}

func normalizeEndpoint(raw string)(string,error){raw=strings.TrimSpace(raw);parsed,err:=url.Parse(raw);if err!=nil||parsed.Scheme!="https"||parsed.Host==""||parsed.User!=nil||parsed.Path!=""&&parsed.Path!="/"||parsed.RawQuery!=""||parsed.Fragment!=""{return "",errors.New("remote: endpoint must be an origin-only HTTPS URL")};return parsed.Scheme+"://"+parsed.Host,nil}

func stateOf(profile *Profile,connected bool,detail string)State{if profile==nil{return State{}};return State{Configured:true,Active:profile.Active,Endpoint:profile.Endpoint,ServerName:profile.ServerName,Connected:connected,Detail:detail}}

func readProfile(path string)(*Profile,error){info,err:=os.Lstat(path);if err!=nil{return nil,err};if !info.Mode().IsRegular()||info.Mode().Perm()!=0o600||info.Size()>16<<10{return nil,errors.New("remote: profile must be a bounded 0600 regular file")};contents,err:=os.ReadFile(path);if err!=nil{return nil,err};var profile Profile;decoder:=json.NewDecoder(bytes.NewReader(contents));decoder.DisallowUnknownFields();if err:=decoder.Decode(&profile);err!=nil{return nil,err};if err:=decoder.Decode(&struct{}{});!errors.Is(err,io.EOF){return nil,errors.New("remote: profile contains trailing JSON")};endpoint,err:=normalizeEndpoint(profile.Endpoint);if err!=nil||endpoint!=profile.Endpoint||strings.TrimSpace(profile.ServerName)==""{return nil,errors.New("remote: invalid saved profile")};return &profile,nil}

func writeProfile(path string,profile Profile)error{contents,err:=json.Marshal(profile);if err!=nil{return err};if err:=os.MkdirAll(filepath.Dir(path),0o700);err!=nil{return err};candidate,err:=os.CreateTemp(filepath.Dir(path),".remote-runtime-*");if err!=nil{return err};candidatePath:=candidate.Name();defer os.Remove(candidatePath);if err:=candidate.Chmod(0o600);err!=nil{_=candidate.Close();return err};if _,err:=candidate.Write(contents);err!=nil{_=candidate.Close();return err};if err:=candidate.Sync();err!=nil{_=candidate.Close();return err};if err:=candidate.Close();err!=nil{return err};return os.Rename(candidatePath,path)}
