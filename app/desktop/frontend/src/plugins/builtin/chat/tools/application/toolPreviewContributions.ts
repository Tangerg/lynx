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

export function taskToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["task", "subagent"]);
}

// Background-shell tools all return terminal-style plain text.
export function shellToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["shell", "run_in_background", "shell_output", "shell_kill"]);
}

export function webSearchToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["web_search"]);
}
