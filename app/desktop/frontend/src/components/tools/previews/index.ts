// Shared data types for tool preview components. The concrete previews
// (Bash / Diff / Grep / File) live in `plugins/builtin/tool-previews/` and
// import `PreviewFoot` directly from its source file.

export type { DiffRow, FileLine, GrepMatch, TermLine } from "./types";
