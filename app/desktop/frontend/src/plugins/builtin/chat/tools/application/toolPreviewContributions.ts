import type { ToolPreviewComponent } from "@/plugins/sdk";

export interface ToolPreviewContribution {
  key: string;
  component: ToolPreviewComponent;
}

type PreviewMap<Key extends string> = Readonly<Record<Key, ToolPreviewComponent>>;

function toolPreviews<Key extends string>(components: PreviewMap<Key>): ToolPreviewContribution[] {
  return Object.entries<ToolPreviewComponent>(components).map(([key, component]) => ({
    key,
    component,
  }));
}

export function askUserToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ ask_user: component });
}

export function applyPatchToolPreview(
  components: PreviewMap<"apply_patch">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function fileToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ read: component });
}

export function globToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ glob: component });
}

export function grepToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ grep: component });
}

// One operation-dispatched tool: diagnostics is an `operation` of `lsp`, and the
// runtime asserts no separate lsp_diagnostics coexists with it. The preview reads
// the operation to decide which face to wear.
export function lspToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ lsp: component });
}

// The Skill family: a catalog listing, a loaded Skill's instructions, one bundled
// resource, and a proposal's own body — all name+text results the same list renders.
export function skillToolPreviews(
  components: PreviewMap<"list_skills" | "load_skill" | "read_skill_resource" | "propose_skill">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function delegationToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ delegate_task: component });
}

// The whole shell family returns terminal-style plain text. Backgrounding is an
// ARGUMENT of `shell` (run_in_background), not a tool of its own — read_shell_output
// / stop_shell are how you then read and stop it.
export function shellToolPreviews(
  components: PreviewMap<"shell" | "read_shell_output" | "stop_shell">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function webSearchToolPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ web_search: component });
}

// Searching the agent's own history: project memory and earlier conversations.
// Two shapes, one family — both answer "here is what I already knew".
export function recallToolPreviews(
  components: PreviewMap<"search_memory" | "search_conversations" | "read_tool_result">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function toolSearchPreview(component: ToolPreviewComponent): ToolPreviewContribution[] {
  return toolPreviews({ search_tools: component });
}

export function planToolPreviews(
  components: PreviewMap<"enter_plan_mode" | "set_plan" | "exit_plan_mode">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function goalToolPreviews(
  components: PreviewMap<"create_goal" | "get_goal" | "report_goal_outcome">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function scheduleToolPreviews(
  components: PreviewMap<"create_schedule" | "list_schedules" | "delete_schedule">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}

export function httpToolPreviews(
  components: PreviewMap<"http_request" | "web_fetch">,
): ToolPreviewContribution[] {
  return toolPreviews(components);
}
