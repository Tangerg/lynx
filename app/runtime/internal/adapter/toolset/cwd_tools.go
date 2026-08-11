package toolset

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"
)

// buildCWDTools instantiates the working-directory-bound filesystem capabilities,
// all anchored at cwd. These are the only tools whose behavior depends on
// the working directory, so they are rebuilt per resolution (cheap structs)
// rather than captured once. The filesystem tools need no credentials. (The
// shell family is built over shared exec.Shells in shell.Build, not here; it
// reads cwd per call.)
//
// apply_patch is wrapped so a successful mutation is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result. ci may be nil — the wrap is then a no-op.
// locker is owner-scoped: resolver-owned builds reuse one locker so
// read/check/mutate stays atomic across concurrent Runs, not merely across the
// tools resolved for one Run.
type cwdTools struct {
	readSearch []toolcontract.Tool
	applyPatch toolcontract.Tool
}

func buildCWDTools(cwd string, ci *codeintel.Analyzer, tracker *readTracker, locker *pathLocker) cwdTools {
	fsExec := fs.NewLocalExecutor(cwd)

	// Guard stack, innermost → outermost: auto-format the applied
	// change; diagnostics type-check it; read/staleness guard gates before the
	// change and refreshes the read stamp after; per-path lock serializes
	// concurrent mutations to the same file; path guard refuses protected dirs.
	applyPatch := guardedMutation(fs.NewApplyPatchTool(fsExec), ci, tracker, locker, cwd)

	families := cwdTools{
		readSearch: []toolcontract.Tool{
			withPathLock(withReadTracking(fs.NewReadTool(fsExec), tracker, cwd), locker, cwd),
			fs.NewGlobTool(fsExec),
			fs.NewGrepTool(fsExec),
		},
		applyPatch: applyPatch,
	}
	return families
}

func guardedMutation(tool toolcontract.Tool, ci *codeintel.Analyzer, tracker *readTracker, locker *pathLocker, cwd string) toolcontract.Tool {
	return withPathGuard(
		withPathLock(
			withMutationGuard(
				withMutationRecording(
					withMutationDiagnostics(
						withAutoFormat(tool, cwd),
						ci,
						cwd,
					),
				),
				tracker,
				cwd,
			),
			locker,
			cwd,
		),
		cwd,
	)
}
