// Built-in tool previews — one file per tool family, each a small React
// component + a `host.extensions.contribute(TOOL_PREVIEW, …)` plugin. They use
// the same SDK surface third-party plugins do (no special-casing: a new tool fn
// means a new preview plugin). This barrel only aggregates the specs for the
// manifest.

// Generic previews.
export { shellPreview } from "./terminal";
export { diff } from "./diff";
export { file } from "./file";
export { grep } from "./grep";

// Specialised previews — one file per tool family.
export { askUserPreview } from "./askUser";
export { globPreview } from "./glob";
export { lspPreviews } from "./lsp";
export { skillPreview } from "./skill";
export { taskPreview } from "./task";
export { webSearchPreview } from "./webSearch";
