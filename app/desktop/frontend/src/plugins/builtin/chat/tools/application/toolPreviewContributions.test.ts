import { describe, expect, it } from "vitest";
import {
  askUserToolPreview,
  diffToolPreviews,
  fileToolPreview,
  globToolPreview,
  grepToolPreview,
  lspToolPreview,
  shellToolPreviews,
  skillToolPreviews,
  delegationToolPreview,
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

  // Diagnostics is an `operation` of the one `lsp` tool, not a tool of its own, so
  // there is a single registry key and the preview dispatches on the operation.
  it("registers one key for the operation-dispatched LSP tool", () => {
    expect(lspToolPreview(Preview)).toEqual([{ key: "lsp", component: Preview }]);
  });
});
