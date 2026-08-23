package codeintel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (service *Service) Query(ctx context.Context, root string, request Request) (string,error) {
	if !filepath.IsAbs(root) || filepath.Clean(root)!=root { return "",errors.New("codeintel: workspace must be canonical and absolute") }
	if request.Operation=="workspace_symbols" { return service.workspaceSymbols(ctx,root,request.Query) }
	path,err:=confinedPath(root,request.Path)
	if err!=nil { return "",err }
	client,err:=service.clientFor(ctx,root,path)
	if err!=nil { return "",err }
	uri,diagnosticSignal,changed,err:=client.sync(ctx,path)
	if err!=nil { return "",err }
	position:=map[string]int{"line":request.Line-1,"character":request.Character-1}
	params:=map[string]any{"textDocument":map[string]string{"uri":uri},"position":position}
	switch request.Operation {
	case "definition","implementation","references":
		method:="textDocument/"+request.Operation
		if request.Operation=="references" { params["context"]=map[string]bool{"includeDeclaration":true} }
		var raw json.RawMessage
		if err:=client.connection.Call(ctx,method,params,&raw); err!=nil { return "",fmt.Errorf("codeintel: %s: %w",request.Operation,err) }
		locations,err:=decodeLocations(raw); if err!=nil{return "",err}; return formatLocations(root,locations,request.Operation),nil
	case "hover":
		var raw json.RawMessage; if err:=client.connection.Call(ctx,"textDocument/hover",params,&raw);err!=nil{return "",err}; return decodeHover(raw)
	case "document_symbols":
		var raw json.RawMessage; if err:=client.connection.Call(ctx,"textDocument/documentSymbol",map[string]any{"textDocument":map[string]string{"uri":uri}},&raw);err!=nil{return "",err}
		symbols,err:=decodeSymbols(raw,uri);if err!=nil{return "",err};return formatSymbols(root,symbols),nil
	case "incoming_calls","outgoing_calls":
		var prepared []callItem; if err:=client.connection.Call(ctx,"textDocument/prepareCallHierarchy",params,&prepared);err!=nil{return "",err};if len(prepared)==0{return "No calls found.",nil}
		method:="callHierarchy/incomingCalls";if request.Operation=="outgoing_calls"{method="callHierarchy/outgoingCalls"}
		var calls []callEdge;if err:=client.connection.Call(ctx,method,map[string]any{"item":prepared[0]},&calls);err!=nil{return "",err}
		values:=make([]symbol,0,len(calls));for _,edge:=range calls{item:=edge.From;if request.Operation=="outgoing_calls"{item=edge.To};values=append(values,item.symbol())};return formatSymbols(root,values),nil
	case "diagnostics":
		return client.diagnosticResult(ctx,uri,request.Path,diagnosticSignal,changed),nil
	default:return "",fmt.Errorf("codeintel: unknown operation %q",request.Operation)
	}
}

func confinedPath(root,path string)(string,error){
	if strings.TrimSpace(path)==""{return "",errors.New("codeintel: path is required")};if !filepath.IsAbs(path){path=filepath.Join(root,path)};path=filepath.Clean(path)
	relative,err:=filepath.Rel(root,path);if err!=nil||relative==".."||strings.HasPrefix(relative,".."+string(filepath.Separator)){return "",errors.New("codeintel: path escapes the workspace")}
	info,err:=os.Stat(path);if err!=nil{return "",err};if !info.Mode().IsRegular(){return "",errors.New("codeintel: target is not a regular file")};return path,nil
}

func (service *Service) workspaceSymbols(ctx context.Context,root,query string)(string,error){
	if strings.TrimSpace(query)==""{return "",errors.New("codeintel: workspace symbol query is required")};var values []symbol;var errs []error;successful:=false
	for _,spec:=range service.specs{applies:=false;for _,marker:=range spec.RootMarkers{if _,err:=os.Stat(filepath.Join(root,marker));err==nil{applies=true;break}};if !applies{continue};client,err:=service.server(ctx,root,spec);if err!=nil{errs=append(errs,err);continue};var raw json.RawMessage;if err:=client.connection.Call(ctx,"workspace/symbol",map[string]string{"query":query},&raw);err!=nil{errs=append(errs,err);continue};symbols,err:=decodeSymbols(raw,"");if err!=nil{errs=append(errs,err);continue};successful=true;values=append(values,symbols...)}
	if len(values)==0&&successful{return "No symbols found.",nil};if len(values)==0&&len(errs)>0{return "",errors.Join(errs...)};if len(values)==0{return "",ErrUnsupported};return formatSymbols(root,values),nil
}

func (value *client) diagnosticResult(ctx context.Context,uri,path string,signal <-chan struct{},changed bool)string{
	diagnostics:=value.awaitDiagnostics(ctx,uri,signal,changed);if len(diagnostics)==0{return fmt.Sprintf("No diagnostics for %s.",path)};return formatDiagnostics(path,diagnostics)
}

func(value *client)awaitDiagnostics(ctx context.Context,uri string,signal <-chan struct{},changed bool)[]diagnostic{value.mu.Lock();current,present:=value.diagnostics[uri];if !changed&&present{result:=append([]diagnostic(nil),current...);value.mu.Unlock();return result};value.mu.Unlock();timer:=time.NewTimer(3*time.Second);defer timer.Stop();select{case<-signal:case<-timer.C:case<-ctx.Done():};value.mu.Lock();defer value.mu.Unlock();return append([]diagnostic(nil),value.diagnostics[uri]...)}

func formatDiagnostics(path string,diagnostics []diagnostic)string{var result strings.Builder;for _,item:=range diagnostics{severity:=map[int]string{1:"error",2:"warning",3:"info",4:"hint"}[item.Severity];if severity==""{severity="note"};fmt.Fprintf(&result,"%s %s:%d:%d: %s",severity,path,item.Range.Start.Line+1,item.Range.Start.Character+1,item.Message);if item.Source!=""{fmt.Fprintf(&result," [%s]",item.Source)};result.WriteByte('\n')};return strings.TrimSpace(result.String())}

type locationLink struct{TargetURI string `json:"targetUri"`;TargetSelectionRange wireRange `json:"targetSelectionRange"`}
func decodeLocations(raw json.RawMessage)([]location,error){raw=bytes.TrimSpace(raw);if len(raw)==0||bytes.Equal(raw,[]byte("null")){return nil,nil};var values []json.RawMessage;if raw[0]=='{'{values=[]json.RawMessage{raw}}else if err:=json.Unmarshal(raw,&values);err!=nil{return nil,err};result:=make([]location,0,len(values));for _,value:=range values{var direct location;if json.Unmarshal(value,&direct)==nil&&direct.URI!=""{result=append(result,direct);continue};var link locationLink;if err:=json.Unmarshal(value,&link);err!=nil||link.TargetURI==""{return nil,errors.New("codeintel: malformed location response")};result=append(result,location{URI:link.TargetURI,Range:link.TargetSelectionRange})};return result,nil}

type symbolInfo struct{Name string `json:"name"`;Kind int `json:"kind"`;Location *location `json:"location"`;Container string `json:"containerName"`;Detail string `json:"detail"`;Range *wireRange `json:"range"`;SelectionRange *wireRange `json:"selectionRange"`;Children []symbolInfo `json:"children"`}
func decodeSymbols(raw json.RawMessage,documentURI string)([]symbol,error){raw=bytes.TrimSpace(raw);if len(raw)==0||bytes.Equal(raw,[]byte("null")){return nil,nil};var values []symbolInfo;if err:=json.Unmarshal(raw,&values);err!=nil{return nil,err};var result []symbol;var appendValues func([]symbolInfo,string)error;appendValues=func(items []symbolInfo,parent string)error{for _,item:=range items{if item.Name==""{return errors.New("codeintel: symbol has no name")};position:=item.Location;if position==nil&&item.SelectionRange!=nil{position=&location{URI:documentURI,Range:*item.SelectionRange}};if position==nil{return errors.New("codeintel: symbol has no location")};result=append(result,symbol{Name:item.Name,Kind:item.Kind,Location:*position,Container:firstNonempty(item.Container,parent),Detail:item.Detail});if err:=appendValues(item.Children,item.Name);err!=nil{return err}};return nil};return result,appendValues(values,"")}

func decodeHover(raw json.RawMessage)(string,error){raw=bytes.TrimSpace(raw);if len(raw)==0||bytes.Equal(raw,[]byte("null")){return "No hover information found.",nil};var envelope struct{Contents json.RawMessage `json:"contents"`};if err:=json.Unmarshal(raw,&envelope);err!=nil{return "",err};return flattenMarkup(envelope.Contents),nil}
func flattenMarkup(raw json.RawMessage)string{var text string;if json.Unmarshal(raw,&text)==nil{return text};var value struct{Value string `json:"value"`};if json.Unmarshal(raw,&value)==nil&&value.Value!=""{return value.Value};var values []json.RawMessage;if json.Unmarshal(raw,&values)==nil{parts:=make([]string,0,len(values));for _,item:=range values{if part:=flattenMarkup(item);part!=""{parts=append(parts,part)}};return strings.Join(parts,"\n\n")};return string(raw)}

type callItem struct{Name string `json:"name"`;Kind int `json:"kind"`;URI string `json:"uri"`;SelectionRange wireRange `json:"selectionRange"`;Detail string `json:"detail"`;Data json.RawMessage `json:"data,omitempty"`}
func(item callItem)symbol()symbol{return symbol{Name:item.Name,Kind:item.Kind,Location:location{URI:item.URI,Range:item.SelectionRange},Detail:item.Detail}}
type callEdge struct{From callItem `json:"from"`;To callItem `json:"to"`}

func formatLocations(root string,values []location,kind string)string{if len(values)==0{return "No "+strings.ReplaceAll(kind,"_"," ")+" found."};var result strings.Builder;for _,value:=range values{fmt.Fprintf(&result,"%s:%d:%d\n",relativePath(root,uriPath(value.URI)),value.Range.Start.Line+1,value.Range.Start.Character+1)};return strings.TrimSpace(result.String())}
func formatSymbols(root string,values []symbol)string{if len(values)==0{return "No symbols found."};var result strings.Builder;for _,value:=range values{kind:=symbolKinds[value.Kind];if kind==""{kind="symbol"};fmt.Fprintf(&result,"%s %s",kind,value.Name);if value.Detail!=""{fmt.Fprintf(&result," %s",value.Detail)};if value.Container!=""{fmt.Fprintf(&result," (in %s)",value.Container)};fmt.Fprintf(&result," — %s:%d:%d\n",relativePath(root,uriPath(value.Location.URI)),value.Location.Range.Start.Line+1,value.Location.Range.Start.Character+1)};return strings.TrimSpace(result.String())}
func firstNonempty(values ...string)string{for _,value:=range values{if value!=""{return value}};return ""}
