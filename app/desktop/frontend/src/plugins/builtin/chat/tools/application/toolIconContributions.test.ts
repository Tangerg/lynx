import { describe, expect, it } from "vitest";
import {
  DEFAULT_TOOL_ICONS,
  defaultToolIconContributions,
  defaultToolIconFor,
} from "./toolIconContributions";

const entries = (items: { key: string; icon: string }[]) =>
  Object.fromEntries(items.map((item) => [item.key, item.icon]));

describe("tool icon contributions", () => {
  it("maps built-in tool keys to their domain glyphs", () => {
    expect(DEFAULT_TOOL_ICONS).toMatchObject({
      shell: "terminal",
      run_in_background: "play",
      shell_output: "list",
      shell_kill: "stop",
      read: "eye",
      write: "file-plus",
      edit: "edit",
      grep: "search",
      glob: "folder-search",
      web_search: "globe",
      web_fetch: "download",
      lsp: "code",
      lsp_diagnostics: "bug",
      skill: "sparkle",
      task: "spark",
      subagent: "bot",
      ask_user: "question",
    });
  });

  it("turns the default icon table into registry contributions", () => {
    expect(entries(defaultToolIconContributions())).toEqual(DEFAULT_TOOL_ICONS);
  });

  it("falls back by tool family before using the generic tool glyph", () => {
    expect(defaultToolIconFor("lsp_references")).toBe("code");
    expect(defaultToolIconFor("unknown_tool")).toBe("tool");
  });
});
