// Command contractgen writes the machine-readable projection of the Contract
// Registry.
//
// It lives OUTSIDE the ring architecture on purpose (vNext plan D3): it reads the
// delivery ring's method and shape specs and the application ring's system
// invariants, which no runtime component may do in that combination. A code
// generator is not in the runtime's import graph, so the layering rule — which
// constrains that graph — does not apply to it.
//
// Run it through `go generate ./...`. CI's drift gate reruns it and fails on a
// worktree diff (contract §11.4 gate 1): that is the only mechanism that notices
// when the code and the published contract stop agreeing.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// The two union types the manifest projects extra policy from. Kept as vars so
// the projection compares identity rather than matching on a name string.
var (
	streamEventType    = reflect.TypeFor[protocol.StreamEvent]()
	workspaceEventType = reflect.TypeFor[protocol.WorkspaceEvent]()
)

func main() {
	out := flag.String("out", ".", "directory the Go-side artifacts are written to")
	ts := flag.String("ts", "", "directory the TypeScript wire types are written to; skipped when empty")
	flag.Parse()

	if err := run(*out, *ts); err != nil {
		fmt.Fprintln(os.Stderr, "contractgen:", err)
		os.Exit(1)
	}
}

func run(dir, tsDir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	// Built once: every artifact must describe the same registry snapshot, and a
	// second build would let them disagree if anything about it were not pure.
	registry, shapes := dispatch.Contract(), dispatch.WireShapes()
	walked := walkWireTypes(registry, shapes)
	built := build(walked)

	for _, artifact := range []struct {
		name    string
		content any
	}{
		{"manifest.json", built},
		{"schema.json", newBundle(walked)},
		{"openrpc.json", newOpenRPC(registry, walked)},
	} {
		if err := writeJSON(filepath.Join(dir, artifact.name), artifact.content); err != nil {
			return err
		}
	}
	path := filepath.Join(dir, "API_REFERENCE.md")
	if err := os.WriteFile(path, []byte(reference(built)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if tsDir == "" {
		return nil
	}
	path = filepath.Join(tsDir, tsFileName)
	if err := os.WriteFile(path, []byte(newTypeScript(walked)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeJSON writes an artifact indented with a trailing newline, so a reviewer
// reads a diff rather than one long line — the drift gate's output is meant to be
// read by a person, not just compared.
func writeJSON(path string, content any) error {
	encoded, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
