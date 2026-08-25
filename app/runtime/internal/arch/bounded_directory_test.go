package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestProductionDirectoryReadsAreFinite prevents a complete filesystem
// directory from being materialized before a Runtime resource boundary can
// apply cancellation or capacity policy. Production callers must either read a
// positive batch through (*os.File).ReadDir or use a stricter bounded source.
func TestProductionDirectoryReadsAreFinite(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "ReadDir" || len(call.Args) != 1 {
				return true
			}
			if owner, ok := selector.X.(*ast.Ident); ok && owner.Name == "os" {
				t.Errorf("%s: os.ReadDir materializes a complete directory; use a positive bounded batch", fset.Position(call.Pos()))
				return true
			}
			if readDirLimitIsNonPositive(call.Args[0]) {
				t.Errorf("%s: ReadDir with a non-positive limit materializes a complete directory", fset.Position(call.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCheckpointGitUsesBoundedProcessOwner keeps workspace rollback on the
// same stdout/stderr/lifetime boundary as every other Runtime Git observation.
// Checkpoint owns snapshot semantics, not a second exec.Cmd buffer lifecycle.
func TestCheckpointGitUsesBoundedProcessOwner(t *testing.T) {
	file := filepath.Join(moduleRoot(t), "internal", "infra", "checkpoint", "git.go")
	forbidExternalImports(t, file, []string{"bytes"})
	forbidSelectorCalls(t, file, map[string]string{
		"CommandContext": "checkpoint Git commands must use gitprocess.Run's bounded process lifecycle",
	})
}

func readDirLimitIsNonPositive(expression ast.Expr) bool {
	sign := int64(1)
	if unary, ok := expression.(*ast.UnaryExpr); ok {
		if unary.Op != token.SUB {
			return false
		}
		sign = -1
		expression = unary.X
	}
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return false
	}
	value, err := strconv.ParseInt(literal.Value, 0, 64)
	return err == nil && sign*value <= 0
}
