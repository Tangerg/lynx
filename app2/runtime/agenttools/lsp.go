package agenttools

import (
	"context"
	"errors"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/codeintel"
)

type lspPathResolver interface {
	Root() string
	AbsolutePath(string) (string, error)
}

type lspRequest struct {
	Operation string `json:"operation" jsonschema:"enum=definition,enum=references,enum=implementation,enum=hover,enum=incoming_calls,enum=outgoing_calls,enum=document_symbols,enum=workspace_symbols,enum=diagnostics"`
	Path string `json:"path,omitempty"`
	Line int `json:"line,omitempty" jsonschema:"minimum=1"`
	Character int `json:"character,omitempty" jsonschema:"minimum=1"`
	Query string `json:"query,omitempty"`
}

func newLSPTool(service *codeintel.Service, paths lspPathResolver)(toolcontract.Tool,error){
	if service==nil||paths==nil{return nil,errors.New("agenttools: code intelligence service and path resolver are required")}
	return toolcontract.NewFunc(toolcontract.FuncConfig{Name:"lsp",Description:"Query a workspace language server. Position operations use path plus 1-based line and character; document_symbols and diagnostics use path; workspace_symbols uses query."},func(ctx context.Context,request lspRequest)(string,error){
		position:=map[string]bool{"definition":true,"references":true,"implementation":true,"hover":true,"incoming_calls":true,"outgoing_calls":true}[request.Operation]
		switch{case position&&(strings.TrimSpace(request.Path)==""||request.Line<1||request.Character<1):return "",errors.New("lsp: position operations require path, line, and character")
		case (request.Operation=="document_symbols"||request.Operation=="diagnostics")&&strings.TrimSpace(request.Path)=="":return "",errors.New("lsp: operation requires path")
		case request.Operation=="workspace_symbols"&&strings.TrimSpace(request.Query)=="":return "",errors.New("lsp: workspace_symbols requires query")}
		if request.Path!=""{resolved,err:=paths.AbsolutePath(request.Path);if err!=nil{return "",err};request.Path=resolved}
		result,err:=service.Query(ctx,paths.Root(),codeintel.Request{Operation:request.Operation,Path:request.Path,Line:request.Line,Character:request.Character,Query:request.Query})
		if errors.Is(err,codeintel.ErrUnsupported)||errors.Is(err,codeintel.ErrUnavailable){return "No language server is available for that file type or workspace.",nil}
		return result,err
	})
}
