package main

import (
	stderrors "errors"
	"fmt"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

const runtimeModulePath = "github.com/Tangerg/lynx/app/runtime"

var publicPackagePaths = []string{
	runtimeModulePath + "/embedded",
	runtimeModulePath + "/localruntime",
	runtimeModulePath + "/protocol",
}

type publicGoAPI struct {
	Packages []publicGoPackage `json:"packages"`
}

type publicGoPackage struct {
	Path      string             `json:"path"`
	Imports   []string           `json:"imports,omitempty"`
	Constants []publicGoConstant `json:"constants,omitempty"`
	Variables []publicGoVariable `json:"variables,omitempty"`
	Functions []publicGoFunction `json:"functions,omitempty"`
	Types     []publicGoType     `json:"types,omitempty"`
}

type publicGoConstant struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type publicGoVariable struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type publicGoFunction struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

type publicGoType struct {
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	Underlying string             `json:"underlying,omitempty"`
	Fields     []publicGoField    `json:"fields,omitempty"`
	Methods    []publicGoFunction `json:"methods,omitempty"`
}

type publicGoField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

func loadPublicGoAPI() (publicGoAPI, error) {
	configuration := &packages.Config{Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedTypesSizes}
	loaded, err := packages.Load(configuration, publicPackagePaths...)
	if err != nil {
		return publicGoAPI{}, fmt.Errorf("load public Go packages: %w", err)
	}
	var loadErrors []error
	for _, pkg := range loaded {
		for _, packageError := range pkg.Errors {
			loadErrors = append(loadErrors, stderrors.New(packageError.Error()))
		}
	}
	if err := stderrors.Join(loadErrors...); err != nil {
		return publicGoAPI{}, fmt.Errorf("load public Go packages: %w", err)
	}
	if len(loaded) != len(publicPackagePaths) {
		return publicGoAPI{}, fmt.Errorf("loaded %d public Go packages, want %d", len(loaded), len(publicPackagePaths))
	}

	api := publicGoAPI{Packages: make([]publicGoPackage, 0, len(loaded))}
	for _, pkg := range loaded {
		projection, err := projectPublicGoPackage(pkg)
		if err != nil {
			return publicGoAPI{}, err
		}
		api.Packages = append(api.Packages, projection)
	}
	slices.SortFunc(api.Packages, func(left, right publicGoPackage) int {
		return strings.Compare(left.Path, right.Path)
	})
	return api, nil
}

func projectPublicGoPackage(pkg *packages.Package) (publicGoPackage, error) {
	projection := publicGoPackage{Path: pkg.PkgPath}
	imports := make(map[string]struct{})
	qualifier := func(imported *types.Package) string {
		if imported == nil || imported.Path() == pkg.PkgPath {
			return ""
		}
		imports[imported.Path()] = struct{}{}
		return imported.Name()
	}
	typeString := func(value types.Type) string { return types.TypeString(value, qualifier) }

	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		object := scope.Lookup(name)
		if !object.Exported() {
			continue
		}
		switch value := object.(type) {
		case *types.Const:
			projection.Constants = append(projection.Constants, publicGoConstant{
				Name: value.Name(), Type: typeString(value.Type()), Value: value.Val().ExactString(),
			})
		case *types.Var:
			projection.Variables = append(projection.Variables, publicGoVariable{Name: value.Name(), Type: typeString(value.Type())})
		case *types.Func:
			projection.Functions = append(projection.Functions, publicGoFunction{Name: value.Name(), Signature: typeString(value.Type())})
		case *types.TypeName:
			projection.Types = append(projection.Types, projectPublicGoType(value, typeString))
		}
	}
	for imported := range imports {
		if strings.Contains(imported, "/internal/") {
			return publicGoPackage{}, fmt.Errorf("public package %s exposes internal package %s", pkg.PkgPath, imported)
		}
		projection.Imports = append(projection.Imports, imported)
	}
	slices.Sort(projection.Imports)
	return projection, nil
}

func projectPublicGoType(object *types.TypeName, typeString func(types.Type) string) publicGoType {
	projection := publicGoType{Name: object.Name()}
	if object.IsAlias() {
		projection.Kind = "alias"
		projection.Underlying = typeString(types.Unalias(object.Type()))
		return projection
	}
	named := object.Type().(*types.Named)
	switch underlying := named.Underlying().(type) {
	case *types.Struct:
		projection.Kind = "struct"
		for index := range underlying.NumFields() {
			field := underlying.Field(index)
			if !field.Exported() {
				continue
			}
			projection.Fields = append(projection.Fields, publicGoField{
				Name: field.Name(), Type: typeString(field.Type()), Tag: underlying.Tag(index),
			})
		}
	case *types.Interface:
		projection.Kind = "interface"
	default:
		projection.Kind = "defined"
		projection.Underlying = typeString(underlying)
	}

	methodReceiver := types.Type(types.NewPointer(named))
	if _, isInterface := named.Underlying().(*types.Interface); isInterface {
		methodReceiver = named
	}
	methods := types.NewMethodSet(methodReceiver)
	for index := range methods.Len() {
		method := methods.At(index).Obj()
		if !method.Exported() {
			continue
		}
		projection.Methods = append(projection.Methods, publicGoFunction{
			Name: method.Name(), Signature: typeString(method.Type()),
		})
	}
	slices.SortFunc(projection.Methods, func(left, right publicGoFunction) int {
		return strings.Compare(left.Name, right.Name)
	})
	return projection
}
