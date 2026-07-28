import type { ToolPreviewComponent } from "@/plugins/sdk";

export interface ToolPreviewContribution {
  key: string;
  component: ToolPreviewComponent;
}

function toolPreviews(component: ToolPreviewComponent, keys: string[]): ToolPreviewContribution[] {
  return keys.map((key) => ({ key, component }));
}

export function askUserToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["ask_user"]);
}

export function diffToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["edit", "write"]);
}

export function fileToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["read"]);
}

export function globToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["glob"]);
}

export function grepToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["grep"]);
}

export function lspToolPreviews(
  lsp: ToolPreviewComponent,
  diagnostics: ToolPreviewComponent,
): ToolPreviewContribution[] {
  return [
    { key: "lsp", component: lsp },
    { key: "lsp_diagnostics", component: diagnostics },
  ];
}

export function skillToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["skill"]);
}

export function taskToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["task"]);
}

// The whole shell family returns terminal-style plain text. Backgrounding is an
// ARGUMENT of `shell` (run_in_background), not a tool of its own — shell_output /
// shell_kill are how you then read and stop it.
export function shellToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["shell", "shell_output", "shell_kill"]);
}

export function webSearchToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["web_search"]);
}
