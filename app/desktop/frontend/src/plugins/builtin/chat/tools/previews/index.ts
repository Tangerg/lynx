// Built-in tool previews — one file per tool family, each a small React
// component + a `host.extensions.contribute(TOOL_PREVIEW, …)` plugin. They use
// the same SDK surface third-party plugins do (no special-casing: a new tool fn
// means a new preview plugin). This barrel only aggregates the specs for the
// manifest.

// Workspace and command previews.
export { shellPreview } from "./terminal";
export { applyPatchPreview } from "./patch";
export { file } from "./file";
export { grep } from "./grep";

// Agent and integration previews.
export { askUserPreview } from "./askUser";
export { globPreview } from "./glob";
export { goalPreviews } from "./goal";
export { httpPreviews } from "./http";
export { lspPreviews } from "./lsp";
export { planPreviews } from "./plan";
export { recallPreviews } from "./recall";
export { schedulePreview } from "./schedule";
export { skillPreview } from "./skill";
export { taskPreview } from "./task";
export { toolSearchPreviewPlugin } from "./toolSearch";
export { webSearchPreview } from "./webSearch";
