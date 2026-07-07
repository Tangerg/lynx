package toolset

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/tools/fs"
)

// BuildWorkdirTools instantiates the working-directory-bound filesystem tools,
// all anchored at workdir. These are the only tools whose behavior depends on
// the working directory, so they are rebuilt per resolution (cheap structs)
// rather than captured once. No credentials needed; safe to build
// unconditionally. (the shell tool is built over the shared exec.Shells in
// shell.Build, not here — it reads cwd per call like shell_output.)
//
// write and edit are wrapped so a successful edit is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result (see withEditDiagnostics). ci may be nil — the wrap is then a no-op.
func BuildWorkdirTools(workdir string, ci *codeintel.Analyzer, tracker *editguard.Tracker) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)

	// write/edit guard stack, innermost → outermost: diagnostics (type-check
	// the applied change) → read/staleness guard (gate before the change,
	// refresh the read stamp after) → per-path lock (serialize concurrent
	// writes to the same file; read-before + write stay atomic) → path guard
	// (refuse writes into protected dirs like .git — checked first). One locker
	// is shared by write + edit so they serialize against each other per path.
	locker := newPathLocker()
	write := withPathGuard(withPathLock(withWriteGuard(withEditDiagnostics(fs.NewWriteTool(fsExec), ci, workdir), tracker, workdir), locker, workdir), workdir)
	edit := withPathGuard(withPathLock(withEditGuard(withEditDiagnostics(fs.NewEditTool(fsExec), ci, workdir), tracker, workdir), locker, workdir), workdir)

	return []chat.Tool{
		withReadTracking(fs.NewReadTool(fsExec), tracker, workdir),
		write,
		edit,
		fs.NewGlobTool(fsExec),
		fs.NewGrepTool(fsExec),
	}
}
