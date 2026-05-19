// Shared utilities for tool preview components.
//
// The concrete previews (Bash / Diff / Grep / File) used to live here as a
// hard-coded set. After the plugin refactor they moved to
// `plugins/builtin/<name>/` and register themselves via the SDK. What stays
// here is the cross-plugin glue: data types + the PreviewFoot pill.

export { PreviewFoot } from "./PreviewFoot";
export type { DiffRow, FileLine, GrepMatch, TermLine } from "./types";
