import { describe, expect, it } from "vitest";
import {
  askUserPreviewAnswer,
  globPreviewData,
  lspPreviewOperation,
  skillPreviewEntries,
  webSearchPreviewResults,
} from "./specialisedPreviewData";
import { parseJsonResult, resultLines } from "./toolResultParsing";

describe("tool result parsing", () => {
  it("returns trimmed result lines and only parses JSON objects", () => {
    expect(resultLines(" a\nb\n\n")).toEqual(["a", "b"]);
    expect(parseJsonResult('{"ok": true}')).toEqual({ ok: true });
    expect(parseJsonResult("[1,2]")).toBeUndefined();
    expect(parseJsonResult("plain")).toBeUndefined();
  });
});

describe("specialisedPreviewData", () => {
  it("parses skill catalog entries", () => {
    expect(
      skillPreviewEntries(
        "<available_skills><skill><name>docs</name><description>Read docs</description></skill></available_skills>",
      ),
    ).toEqual([{ name: "docs", description: "Read docs" }]);
  });

  it("flattens ask_user answer shapes", () => {
    expect(askUserPreviewAnswer("plain answer")).toBe("plain answer");
    expect(askUserPreviewAnswer('{"answer":"yes"}')).toBe("yes");
    expect(askUserPreviewAnswer('{"choices":["red","blue"],"note":"done"}')).toBe(
      "red, blue · done",
    );
  });

  it("uses the same glob key priority as match counting", () => {
    expect(globPreviewData('{"hits":[{"path":"src/a.ts"}],"truncated":true}')).toEqual({
      paths: ["src/a.ts"],
      truncated: true,
    });
    expect(globPreviewData('{"files":["src/b.ts"]}')).toEqual({
      paths: ["src/b.ts"],
      truncated: false,
    });
  });

  it("reads the lsp operation from partial preview args", () => {
    expect(lspPreviewOperation('{"operation":"hover"}')).toBe("hover");
    expect(lspPreviewOperation("{")).toBe("");
  });

  it("projects web search results without depending on UI types", () => {
    expect(
      webSearchPreviewResults(
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
