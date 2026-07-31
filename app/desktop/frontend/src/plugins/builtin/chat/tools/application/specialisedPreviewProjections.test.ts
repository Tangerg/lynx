import { describe, expect, it } from "vitest";
import {
  projectAskUserAnswer,
  projectGlobPreview,
  projectLspOperation,
  projectSkillPreview,
  projectWebSearchPreview,
} from "./specialisedPreviewProjections";
import { parseJsonResult, resultLines } from "./toolResultParsing";

describe("tool result parsing", () => {
  it("returns trimmed result lines and only parses JSON objects", () => {
    expect(resultLines(" a\nb\n\n")).toEqual(["a", "b"]);
    expect(parseJsonResult('{"ok": true}')).toEqual({ ok: true });
    expect(parseJsonResult("[1,2]")).toBeUndefined();
    expect(parseJsonResult("plain")).toBeUndefined();
  });
});

describe("specialised preview projections", () => {
  it("parses skill catalog entries", () => {
    expect(
      projectSkillPreview(
        "<available_skills><skill><name>docs</name><description>Read docs</description></skill></available_skills>",
      ),
    ).toEqual([{ name: "docs", description: "Read docs" }]);
  });

  it("flattens ask_user answer shapes", () => {
    expect(projectAskUserAnswer("plain answer")).toBe("plain answer");
    expect(projectAskUserAnswer('{"answer":"yes"}')).toBe("yes");
    expect(projectAskUserAnswer('{"choices":["red","blue"],"note":"done"}')).toBe(
      "red, blue · done",
    );
  });

  it("uses the same glob key priority as match counting", () => {
    expect(projectGlobPreview('{"hits":[{"path":"src/a.ts"}],"truncated":true}')).toEqual({
      paths: ["src/a.ts"],
      truncated: true,
    });
    expect(projectGlobPreview('{"files":["src/b.ts"]}')).toEqual({
      paths: ["src/b.ts"],
      truncated: false,
    });
  });

  it("reads the lsp operation from partial preview args", () => {
    expect(projectLspOperation('{"operation":"hover"}')).toBe("hover");
    expect(projectLspOperation("{")).toBe("");
  });

  it("projects web search results without depending on UI types", () => {
    expect(
      projectWebSearchPreview(
        '{"results":[{"url":"https://www.example.com/a","title":"Example","snippet":"One"},{"url":""}]}',
      ),
    ).toEqual([
      {
        url: "https://www.example.com/a",
        domain: "example.com",
        title: "Example",
        snippet: "One",
      },
    ]);
  });
});
