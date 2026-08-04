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
  return toolPreviews(component, ["edit", "write", "apply_patch"]);
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

// One operation-dispatched tool: diagnostics is an `operation` of `lsp`, and the
// runtime asserts no separate lsp_diagnostics coexists with it. The preview reads
// the operation to decide which face to wear.
export function lspToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["lsp"]);
}

// The Skill family: a catalog listing, a loaded Skill's instructions, one bundled
// resource, and a proposal's own body — all name+text results the same list renders.
export function skillToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, [
    "list_skills",
    "load_skill",
    "read_skill_resource",
    "propose_skill",
  ]);
}

export function delegationToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["delegate_task"]);
}

// The whole shell family returns terminal-style plain text. Backgrounding is an
// ARGUMENT of `shell` (run_in_background), not a tool of its own — read_shell_output
// / stop_shell are how you then read and stop it.
export function shellToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["shell", "read_shell_output", "stop_shell"]);
}

export function webSearchToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["web_search"]);
}
