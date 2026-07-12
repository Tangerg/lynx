package toolset

import (
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/tools/fs"
	"github.com/Tangerg/lynx/tools/httpreq"
)

// buildWorkdirTools instantiates the working-directory-bound filesystem tools,
// all anchored at workdir. These are the only tools whose behavior depends on
// the working directory, so they are rebuilt per resolution (cheap structs)
// rather than captured once. The filesystem tools need no credentials; the sole
// gated member is download (see below). (the shell tool is built over the
// shared exec.Shells in shell.Build, not here — it reads cwd per call like
// shell_output.)
//
// write and edit are wrapped so a successful edit is type-checked by the
// code-intelligence analyzer and any new problems are folded into the tool
// result (see withEditDiagnostics). ci may be nil — the wrap is then a no-op.
// downloadAllow gates the download tool: empty (no configured host allowlist)
// omits it entirely, so an offline build makes no surprise outbound calls.
//
// locker is owner-scoped: resolver-owned builds reuse one locker so
// read/check/write stays atomic across concurrent turns, not merely across the
// tools resolved for one turn.
func buildWorkdirTools(workdir string, ci *codeintel.Analyzer, tracker *editguard.Tracker, downloadAllow httpreq.Allowlist, locker *pathLocker) []chat.Tool {
	fsExec := fs.NewLocalExecutor(workdir)

	// Mutation guard stack, innermost → outermost: auto-format the applied
	// change; diagnostics type-check it; read/staleness guard gates before the
	// change and refreshes the read stamp after; per-path lock serializes
	// concurrent writes to the same file; path guard refuses protected dirs.
	write := writeMutationTool(fs.NewWriteTool(fsExec), ci, tracker, locker, workdir)
	edit := editMutationTool(fs.NewEditTool(fsExec), ci, tracker, locker, workdir)
	applyPatch := editMutationTool(fs.NewApplyPatchTool(fsExec), ci, tracker, locker, workdir)

	tools := []chat.Tool{
		withPathLock(withReadTracking(fs.NewReadTool(fsExec), tracker, workdir), locker, workdir),
		write,
		edit,
		applyPatch,
		fs.NewGlobTool(fsExec),
		fs.NewGrepTool(fsExec),
	}
	// download fetches an arbitrary URL and writes it to disk — the same SSRF
	// surface as httpreq — so it is registered only when a host allowlist is
	// configured, and enforces that same allowlist per call. No allowlist → the
	// tool is absent, matching the online-tools opt-in.
	if !downloadAllow.Empty() {
		download := withPathGuard(withPathLock(newDownloadTool(workdir, downloadAllow), locker, workdir), workdir)
		tools = append(tools, download)
	}
	return tools
}

func writeMutationTool(tool chat.Tool, ci *codeintel.Analyzer, tracker *editguard.Tracker, locker *pathLocker, workdir string) chat.Tool {
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

func editMutationTool(tool chat.Tool, ci *codeintel.Analyzer, tracker *editguard.Tracker, locker *pathLocker, workdir string) chat.Tool {
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
