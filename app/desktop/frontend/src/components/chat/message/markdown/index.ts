// Markdown rendering sub-module. Public surface: the standalone code block
// (also used outside markdown — e.g. the preview-blocks `code` renderer).
// MarkdownMessage stays internal to the message module (only PartRenderer +
// ReasoningBlock consume it, both siblings); markdownComponents / MermaidBlock
// / HtmlArtifact are private rendering details.

export { ShikiCodeBlock } from "./ShikiCodeBlock";
