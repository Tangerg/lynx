package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
)

func TestPublicGoAPICapturesExactRuntimeBinding(t *testing.T) {
	api, err := loadPublicGoAPI()
	if err != nil {
		t.Fatalf("load public Go API: %v", err)
	}
	if len(api.Packages) != len(publicPackagePaths) {
		t.Fatalf("public packages = %d, want %d", len(api.Packages), len(publicPackagePaths))
	}

	embedded := publicGoPackageByPath(t, api, runtimeModulePath+"/embedded")
	if !slices.ContainsFunc(embedded.Functions, func(function publicGoFunction) bool {
		return function.Name == "Open" && strings.Contains(function.Signature, "(*Runtime, error)")
	}) {
		t.Fatal("embedded public API does not contain Open returning *Runtime")
	}
	if len(embedded.Imports) != 3 || slices.ContainsFunc(embedded.Imports, func(path string) bool {
		return strings.Contains(path, "/internal/")
	}) {
		t.Fatalf("embedded public imports = %v", embedded.Imports)
	}

	runtimeType := publicGoTypeByName(t, embedded, "Runtime")
	if got, want := len(runtimeType.Methods), len(operation.Contract().Metas())+1; got != want {
		t.Fatalf("Runtime methods = %d, want %d operations plus Close", got, want)
	}
	if !slices.ContainsFunc(runtimeType.Methods, func(method publicGoFunction) bool {
		return method.Name == "Close" && method.Signature == "func() error"
	}) {
		t.Fatal("Runtime public API does not contain Close")
	}

	localRuntime := publicGoPackageByPath(t, api, runtimeModulePath+"/localruntime")
	for _, name := range []string{"OpenToken", "ReadToken"} {
		if !slices.ContainsFunc(localRuntime.Functions, func(function publicGoFunction) bool {
			return function.Name == name && function.Signature == "func(path string) (*Token, error)"
		}) {
			t.Fatalf("localruntime public API does not contain %s", name)
		}
	}
	token := publicGoTypeByName(t, localRuntime, "Token")
	for _, name := range []string{"Path", "Value"} {
		if !slices.ContainsFunc(token.Methods, func(method publicGoFunction) bool {
			return method.Name == name && method.Signature == "func() string"
		}) {
			t.Fatalf("localruntime.Token public API does not contain %s", name)
		}
	}

	protocol := publicGoPackageByPath(t, api, runtimeModulePath+"/protocol")
	if len(protocol.Types) == 0 || len(protocol.Constants) == 0 {
		t.Fatal("protocol Go baseline is vacuous")
	}
	problemError := publicGoTypeByName(t, protocol, "ProblemError")
	if !slices.ContainsFunc(problemError.Methods, func(method publicGoFunction) bool {
		return method.Name == "Problem" && method.Signature == "func() ProblemData"
	}) {
		t.Fatal("protocol ProblemError method contract is missing")
	}
}

func publicGoPackageByPath(t *testing.T, api publicGoAPI, path string) publicGoPackage {
	t.Helper()
	for _, pkg := range api.Packages {
		if pkg.Path == path {
			return pkg
		}
	}
	t.Fatalf("public Go API has no package %s", path)
	return publicGoPackage{}
}

func publicGoTypeByName(t *testing.T, pkg publicGoPackage, name string) publicGoType {
	t.Helper()
	for _, declared := range pkg.Types {
		if declared.Name == name {
			return declared
		}
	}
	t.Fatalf("public Go package %s has no type %s", pkg.Path, name)
	return publicGoType{}
}
