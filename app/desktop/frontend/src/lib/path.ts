// Path DISPLAY helpers — how a cwd reads in chrome (chips, tree nodes),
// not filesystem logic. The runtime owns real path semantics (jail,
// normalization); the UI only ever shortens what the server returned.

/** Last segment of a directory path ("/a/b/c/" → "c"); the input itself
 *  when there's nothing to split (root, ""). Uses nullish coalescing (??)
 *  rather than logical OR (||) because an empty-string segment (which
 *  .at(-1) returns for "/") is falsy in JS — `"" || path` would leak
 *  the full path, e.g. basename("/") → "/" instead of "". */
export function basename(path: string): string {
  return path.replace(/\/+$/, "").split("/").at(-1) ?? path;
}
