// Package toolflow exposes the deliberately small read-only diagnostic catalog
// that is safe to invoke outside an Agent Run.
package toolflow

import(
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)
type Resolver interface{Resolve(context.Context,string)(workspacefs.Resolution,error)}
type Service struct{resolver Resolver}
func New(resolver Resolver)(*Service,error){if resolver==nil{return nil,errors.New("toolflow: resolver is required")};return &Service{resolver:resolver},nil}
func(s *Service)List(context.Context)(*protocol.Page[protocol.ToolSpec],error){values:=make([]protocol.ToolSpec,0,3);for _,candidate:=range directTools(""){definition:=candidate.Definition();var schema map[string]any;if err:=json.Unmarshal(definition.InputSchema,&schema);err!=nil{return nil,err};values=append(values,protocol.ToolSpec{Name:definition.Name,Description:definition.Description,Parameters:schema,SafetyClass:protocol.SafetyClassSafe})};return protocol.NewPage(values),nil}
func(s *Service)Invoke(ctx context.Context,request protocol.InvokeToolRequest)(any,error){if request.Workspace==nil{return nil,fmt.Errorf("%w: workspace is required",protocol.ErrInvalidParams)};resolved,err:=s.resolver.Resolve(ctx,request.Workspace.Path);if err!=nil||!resolved.Available{return nil,protocol.ErrWorkspaceUnavailable};encoded,err:=json.Marshal(request.Arguments);if err!=nil{return nil,fmt.Errorf("%w: arguments are invalid",protocol.ErrInvalidParams)};normalized,err:=normalize(resolved.Workspace.Path(),request.Name,encoded);if err!=nil{return nil,err};for _,candidate:=range directTools(resolved.Workspace.Path()){if candidate.Definition().Name!=request.Name{continue};output,err:=candidate.Call(ctx,string(normalized));if err!=nil{return nil,err};var value any;if json.Unmarshal([]byte(output),&value)==nil{return value,nil};return output,nil};return nil,fmt.Errorf("%w: direct tool %q is not registered",protocol.ErrInvalidParams,request.Name)}
func directTools(root string)[]toolcontract.Tool{executor:=fs.NewLocalExecutor(root);return []toolcontract.Tool{fs.NewReadTool(executor),fs.NewGlobTool(executor),fs.NewGrepTool(executor)}}
func normalize(root,name string,encoded []byte)([]byte,error){switch name{case "read":var request fs.ReadRequest;if err:=strict(encoded,&request);err!=nil{return nil,invalid(err)};path,err:=confine(root,request.Path);if err!=nil{return nil,err};request.Path=path;return json.Marshal(request);case "glob":var request fs.GlobRequest;if err:=strict(encoded,&request);err!=nil{return nil,invalid(err)};if filepath.IsAbs(request.Pattern)||slices.Contains(strings.FieldsFunc(request.Pattern,func(r rune)bool{return r=='/'||r==filepath.Separator}),".."){return nil,protocol.ErrPathOutsideRoot};if request.Path!=""{path,err:=confine(root,request.Path);if err!=nil{return nil,err};request.Path=path};return json.Marshal(request);case "grep":var request fs.GrepRequest;if err:=strict(encoded,&request);err!=nil{return nil,invalid(err)};if request.Path!=""{path,err:=confine(root,request.Path);if err!=nil{return nil,err};request.Path=path};return json.Marshal(request);default:return nil,fmt.Errorf("%w: unknown tool",protocol.ErrInvalidParams)}}
func strict(data []byte,target any)error{decoder:=json.NewDecoder(strings.NewReader(string(data)));decoder.DisallowUnknownFields();return decoder.Decode(target)}
func invalid(err error)error{return fmt.Errorf("%w: tool arguments: %v",protocol.ErrInvalidParams,err)}
func confine(root,path string)(string,error){if path==""{return "",fmt.Errorf("%w: path is required",protocol.ErrInvalidParams)};rootAbs,err:=filepath.Abs(root);if err!=nil{return "",err};candidate:=path;if !filepath.IsAbs(candidate){candidate=filepath.Join(rootAbs,candidate)};candidate=filepath.Clean(candidate);relative,err:=filepath.Rel(rootAbs,candidate);if err!=nil||relative==".."||strings.HasPrefix(relative,".."+string(filepath.Separator)){return "",protocol.ErrPathOutsideRoot};physicalParent,err:=filepath.EvalSymlinks(filepath.Dir(candidate));if err==nil{relativeParent,relErr:=filepath.Rel(rootAbs,physicalParent);if relErr!=nil||relativeParent==".."||strings.HasPrefix(relativeParent,".."+string(filepath.Separator)){return "",protocol.ErrPathOutsideRoot}};return filepath.ToSlash(relative),nil}
