package codeintel

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

type ServerSpec struct {
	Name, Command, LanguageID string
	Args, Extensions, RootMarkers []string
}

func DefaultServers() []ServerSpec {
	return []ServerSpec{
		{Name: "gopls", Command: "gopls", LanguageID: "go", Extensions: []string{".go"}, RootMarkers: []string{"go.mod", "go.work"}},
		{Name: "typescript", Command: "typescript-language-server", LanguageID: "typescript", Args: []string{"--stdio"}, Extensions: []string{".ts", ".tsx", ".mts", ".cts"}, RootMarkers: []string{"tsconfig.json", "jsconfig.json", "package.json"}},
	}
}

type Position struct { Line, Character int }
type wirePosition struct { Line int `json:"line"`; Character int `json:"character"` }
type wireRange struct { Start wirePosition `json:"start"`; End wirePosition `json:"end"` }
type location struct { URI string `json:"uri"`; Range wireRange `json:"range"` }
type diagnostic struct {
	Range wireRange `json:"range"`
	Severity int `json:"severity,omitempty"`
	Source string `json:"source,omitempty"`
	Message string `json:"message"`
}
type symbol struct { Name string; Kind int; Location location; Container, Detail string }

type Request struct {
	Operation, Path, Query string
	Line, Character int
}

func pathURI(path string) string {
	value := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && filepath.VolumeName(path) != "" && !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return (&url.URL{Scheme: "file", Path: value}).String()
}
func uriPath(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" { return value }
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func relativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) { return relative }
	return path
}

var symbolKinds = map[int]string{
	1:"file",2:"module",3:"namespace",4:"package",5:"class",6:"method",7:"property",8:"field",9:"constructor",10:"enum",11:"interface",12:"function",13:"variable",14:"constant",15:"string",16:"number",17:"boolean",18:"array",19:"object",20:"key",21:"null",22:"enum-member",23:"struct",24:"event",25:"operator",26:"type-parameter",
}
