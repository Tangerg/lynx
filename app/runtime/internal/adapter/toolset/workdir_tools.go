package toolset

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/editguardstate"
	toolcontract "github.com/Tangerg/lynx/tool"
	"github.com/Tangerg/lynx/tools/fs"
)

// buildWorkdirTools instantiates the working-directory-bound filesystem tools,
// all anchored at workdir. These are the only tools whose behavior depends on
// the working directory, so they are rebuilt per resolution (cheap structs)
// rather than captured once. The filesystem tools need no credentials. (The
// shell tool is built over the
// shared exec.Shells in shell.Build, not here — it reads cwd per call like
// read_shell_output.)
//
// write and edit are wrapped so a successful edit is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result (see withEditDiagnostics). ci may be nil — the wrap is then a no-op.
// locker is owner-scoped: resolver-owned builds reuse one locker so
// read/check/write stays atomic across concurrent turns, not merely across the
// tools resolved for one turn.
type workdirToolFamilies struct {
	readSearch []toolcontract.Tool
	editWrite  []toolcontract.Tool
	applyPatch toolcontract.Tool
}

func buildWorkdirTools(workdir string, ci *codeintel.Analyzer, tracker *editguardstate.Tracker, locker *pathLocker) workdirToolFamilies {
	fsExec := fs.NewLocalExecutor(workdir)

	// Mutation guard stack, innermost → outermost: auto-format the applied
	// change; diagnostics type-check it; read/staleness guard gates before the
	// change and refreshes the read stamp after; per-path lock serializes
	// concurrent writes to the same file; path guard refuses protected dirs.
	write := writeMutationTool(fs.NewWriteTool(fsExec), ci, tracker, locker, workdir)
	edit := editMutationTool(fs.NewEditTool(fsExec), ci, tracker, locker, workdir)
	applyPatch := editMutationTool(fs.NewApplyPatchTool(fsExec), ci, tracker, locker, workdir)

	families := workdirToolFamilies{
		readSearch: []toolcontract.Tool{
			withPathLock(withReadTracking(fs.NewReadTool(fsExec), tracker, workdir), locker, workdir),
			fs.NewGlobTool(fsExec),
			fs.NewGrepTool(fsExec),
		},
		editWrite:  []toolcontract.Tool{edit, write},
		applyPatch: applyPatch,
	}
	return families
}

func writeMutationTool(tool toolcontract.Tool, ci *codeintel.Analyzer, tracker *editguardstate.Tracker, locker *pathLocker, workdir string) toolcontract.Tool {
	return withPathGuard(
		withPathLock(
			withWriteGuard(
				withEditDiagnostics(
					withAutoFormat(tool, workdir),
					ci,
					workdir,
				),
				tracker,
				workdir,
			),
			locker,
			workdir,
		),
		workdir,
	)
}

func editMutationTool(tool toolcontract.Tool, ci *codeintel.Analyzer, tracker *editguardstate.Tracker, locker *pathLocker, workdir string) toolcontract.Tool {
	return withPathGuard(
		withPathLock(
			withEditGuard(
				withEditDiagnostics(
					withAutoFormat(tool, workdir),
					ci,
					workdir,
				),
				tracker,
				workdir,
			),
			locker,
			workdir,
		),
		workdir,
	)
}
