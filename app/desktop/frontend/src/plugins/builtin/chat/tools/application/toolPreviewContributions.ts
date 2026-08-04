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

// Searching the agent's own history: project memory and earlier conversations.
// Two shapes, one family — both answer "here is what I already knew".
export function recallToolPreviews(
  memory: ToolPreviewComponent,
  conversations: ToolPreviewComponent,
): ToolPreviewContribution[] {
  return [
    { key: "search_memory", component: memory },
    { key: "search_conversations", component: conversations },
  ];
}

export function toolSearchPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["search_tools"]);
}

// Only set_plan gets a preview: enter/exit answer in one sentence, and
// exit_plan_mode's row is dropped whenever its question card is present.
export function planToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["set_plan"]);
}

// The three goal operations answer the same { goal, message } envelope, so one
// preview reads all three — what differs is which of them wrote it.
export function goalToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["create_goal", "get_goal", "report_goal_outcome"]);
}

// Creating answers with one schedule, listing with many; the preview renders rows
// either way. delete_schedule answers `{schedule_id}` — a receipt, not a view.
export function scheduleToolPreviews(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews(component, ["create_schedule", "list_schedules"]);
}

export function httpToolPreviews(
  request: ToolPreviewComponent,
  fetch: ToolPreviewComponent,
): ToolPreviewContribution[] {
  return [
    { key: "http_request", component: request },
    { key: "web_fetch", component: fetch },
  ];
}
