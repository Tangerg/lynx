import { describe, expect, it } from "vitest";
import {
  askUserToolPreview,
  diffToolPreviews,
  fileToolPreview,
  globToolPreview,
  grepToolPreview,
  lspToolPreviews,
  shellToolPreviews,
  skillToolPreview,
  taskToolPreviews,
  webSearchToolPreview,
} from "./toolPreviewContributions";

function Preview() {
  return null;
}

function DiagnosticsPreview() {
  return null;
}

const keys = (items: { key: string }[]) => items.map((item) => item.key);

describe("tool preview contributions", () => {
  it("maps specialised preview families to runtime tool keys", () => {
    expect(keys(askUserToolPreview(Preview))).toEqual(["ask_user"]);
    expect(keys(globToolPreview(Preview))).toEqual(["glob"]);
    expect(keys(skillToolPreview(Preview))).toEqual(["skill"]);
    expect(keys(webSearchToolPreview(Preview))).toEqual(["web_search"]);
  });

  it("maps workspace-backed preview families to all supported tool keys", () => {
    expect(keys(diffToolPreviews(Preview))).toEqual(["edit", "write"]);
    expect(keys(fileToolPreview(Preview))).toEqual(["read"]);
    expect(keys(grepToolPreview(Preview))).toEqual(["grep"]);
    expect(keys(shellToolPreviews(Preview))).toEqual([
      "shell",
      "run_in_background",
      "shell_output",
      "shell_kill",
    ]);
    expect(keys(taskToolPreviews(Preview))).toEqual(["task", "subagent"]);
  });

  it("keeps LSP diagnostics on its dedicated renderer", () => {
    expect(lspToolPreviews(Preview, DiagnosticsPreview)).toEqual([
      { key: "lsp", component: Preview },
      { key: "lsp_diagnostics", component: DiagnosticsPreview },
    ]);
  });
});
