import { describe, expect, it } from "vitest";
import { formatToolInspectorBody, toolInspectorModel } from "./toolInspectorModel";

describe("formatToolInspectorBody", () => {
  it("pretty prints structured JSON bodies", () => {
    expect(formatToolInspectorBody('{"path":"src/app.ts","lines":[1,2]}')).toEqual({
      text: '{\n  "path": "src/app.ts",\n  "lines": [\n    1,\n    2\n  ]\n}',
      isJson: true,
    });
  });

  it("preserves raw text and malformed structured text", () => {
    expect(formatToolInspectorBody("plain output")).toEqual({
      text: "plain output",
      isJson: false,
    });
    expect(formatToolInspectorBody("{not json")).toEqual({
      text: "{not json",
      isJson: false,
    });
  });

  it("treats empty or whitespace-only bodies as absent", () => {
    expect(formatToolInspectorBody(undefined)).toEqual({ text: "", isJson: false });
    expect(formatToolInspectorBody(" \n\t ")).toEqual({ text: "", isJson: false });
  });
});

describe("toolInspectorModel", () => {
  it("shows the no-result row only for successful tools with no result body", () => {
    expect(toolInspectorModel({ args: "", status: "ok" }).showNoResult).toBe(true);
    expect(toolInspectorModel({ args: "", status: "running" }).showNoResult).toBe(false);
    expect(toolInspectorModel({ args: "", result: '{"ok":true}', status: "ok" }).showNoResult).toBe(
      false,
    );
  });
});
