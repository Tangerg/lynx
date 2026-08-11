import { describe, expect, it } from "vitest";
import { formatDiagnosticToolResult, parseDiagnosticToolArguments } from "./diagnosticTool";

describe("diagnostic tool input", () => {
  it("accepts a JSON object", () => {
    expect(parseDiagnosticToolArguments('{"path":"README.md","limit":20}')).toEqual({
      ok: true,
      value: { path: "README.md", limit: 20 },
    });
  });

  it("distinguishes invalid JSON from a valid non-object", () => {
    expect(parseDiagnosticToolArguments("{")).toEqual({ ok: false, reason: "invalidJson" });
    expect(parseDiagnosticToolArguments("[]")).toEqual({
      ok: false,
      reason: "objectRequired",
    });
    expect(parseDiagnosticToolArguments("null")).toEqual({
      ok: false,
      reason: "objectRequired",
    });
  });

  it("renders structured and non-JSON results deterministically", () => {
    expect(formatDiagnosticToolResult({ ok: true })).toBe('{\n  "ok": true\n}');
    expect(formatDiagnosticToolResult(undefined)).toBe("undefined");
  });
});
