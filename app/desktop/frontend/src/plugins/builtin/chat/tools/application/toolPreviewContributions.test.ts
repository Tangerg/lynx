import { describe, expect, it } from "vitest";
import { DEFAULT_TOOL_ICONS } from "./toolIconContributions";
import {
  askUserToolPreview,
  diffToolPreviews,
  fileToolPreview,
  globToolPreview,
  grepToolPreview,
  httpToolPreviews,
  lspToolPreview,
  recallToolPreviews,
  scheduleToolPreviews,
  shellToolPreviews,
  skillToolPreviews,
  delegationToolPreview,
  toolSearchPreview,
  webSearchToolPreview,
} from "./toolPreviewContributions";

function Preview() {
  return null;
}

const keys = (items: { key: string }[]) => items.map((item) => item.key);

describe("tool preview contributions", () => {
  it("maps specialised preview families to runtime tool keys", () => {
    expect(keys(askUserToolPreview(Preview))).toEqual(["ask_user"]);
    expect(keys(globToolPreview(Preview))).toEqual(["glob"]);
    expect(keys(skillToolPreviews(Preview))).toEqual([
      "list_skills",
      "load_skill",
      "read_skill_resource",
      "propose_skill",
    ]);
    expect(keys(webSearchToolPreview(Preview))).toEqual(["web_search"]);
  });

  it("maps workspace-backed preview families to all supported tool keys", () => {
    expect(keys(diffToolPreviews(Preview))).toEqual(["edit", "write", "apply_patch"]);
    expect(keys(fileToolPreview(Preview))).toEqual(["read"]);
    expect(keys(grepToolPreview(Preview))).toEqual(["grep"]);
    expect(keys(shellToolPreviews(Preview))).toEqual(["shell", "read_shell_output", "stop_shell"]);
    expect(keys(delegationToolPreview(Preview))).toEqual(["delegate_task"]);
  });

  it("maps the agent's own state and network families to their tool keys", () => {
    expect(keys(recallToolPreviews(Preview, Preview))).toEqual([
      "search_memory",
      "search_conversations",
    ]);
    expect(keys(toolSearchPreview(Preview))).toEqual(["search_tools"]);
    expect(keys(scheduleToolPreviews(Preview))).toEqual(["create_schedule", "list_schedules"]);
    expect(keys(httpToolPreviews(Preview, Preview))).toEqual(["http_request", "web_fetch"]);
  });

  // Diagnostics is an `operation` of the one `lsp` tool, not a tool of its own, so
  // there is a single registry key and the preview dispatches on the operation.
  it("registers one key for the operation-dispatched LSP tool", () => {
    expect(lspToolPreview(Preview)).toEqual([{ key: "lsp", component: Preview }]);
  });

  /**
   * Every tool renders on purpose.
   *
   * A tool with no preview falls to the generic JSON tree, which is a fine answer —
   * but only when someone chose it. Left implicit, a tool added on the backend
   * arrives as an unlabelled blob and nobody notices, which is how `lsp_diagnostics`
   * kept a renderer for a tool that no longer existed. So the decision is a list,
   * and this checks the two lists together cover the whole catalog: adding a tool to
   * the icon table (which every tool needs) without deciding how its result reads
   * fails here.
   */
  it("gives every known tool either a specialised preview or a stated generic one", () => {
    const specialised = new Set(
      keys([
        ...askUserToolPreview(Preview),
        ...diffToolPreviews(Preview),
        ...fileToolPreview(Preview),
        ...globToolPreview(Preview),
        ...grepToolPreview(Preview),
        ...httpToolPreviews(Preview, Preview),
        ...lspToolPreview(Preview),
        ...recallToolPreviews(Preview, Preview),
        ...scheduleToolPreviews(Preview),
        ...shellToolPreviews(Preview),
        ...skillToolPreviews(Preview),
        ...delegationToolPreview(Preview),
        ...toolSearchPreview(Preview),
        ...webSearchToolPreview(Preview),
      ]),
    );
    // Generic BY DECISION: each of these answers in one sentence or a receipt, and a
    // component that wraps one sentence in a panel adds a frame, not information.
    // enter/exit_plan_mode also report through the Plan panel and, for exit, through
    // its own question card.
    const genericByDesign = new Set([
      "enter_plan_mode",
      "exit_plan_mode",
      "delete_schedule",
      "read_tool_result",
      // The plan and goal families report through their own pinned banners, which
      // show what the plan IS and what the allowance has LEFT. A card would answer
      // the same question a second time, frozen at the moment of the call.
      "set_plan",
      "create_goal",
      "get_goal",
      "report_goal_outcome",
    ]);

    const undecided = Object.keys(DEFAULT_TOOL_ICONS).filter(
      (name) => !specialised.has(name) && !genericByDesign.has(name),
    );
    expect(undecided).toEqual([]);
    // And nothing is in both lists — a tool cannot be specialised and deliberately not.
    expect([...genericByDesign].filter((name) => specialised.has(name))).toEqual([]);
  });
});
