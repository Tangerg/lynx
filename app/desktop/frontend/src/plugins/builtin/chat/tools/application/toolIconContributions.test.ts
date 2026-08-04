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
      read_shell_output: "list",
      stop_shell: "stop",
      read: "eye",
      write: "file-plus",
      edit: "edit",
      grep: "search",
      glob: "folder-search",
      web_search: "globe",
      web_fetch: "download",
      lsp: "code",
      list_skills: "sparkle",
      load_skill: "sparkle",
      propose_skill: "sparkle",
      delegate_task: "spark",
      ask_user: "question",
      set_plan: "list",
    });
  });

  it("turns the default icon table into registry contributions", () => {
    expect(entries(defaultToolIconContributions())).toEqual(DEFAULT_TOOL_ICONS);
  });

  it("falls back to the generic tool glyph for a name it does not know", () => {
    expect(defaultToolIconFor("lsp")).toBe("code");
    expect(defaultToolIconFor("acme_do_thing")).toBe("tool");
  });
});
