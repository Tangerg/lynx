import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DiagnosticToolOwner,
  diagnosticToolMaterialGeneration,
  formatDiagnosticToolResult,
  parseDiagnosticToolArguments,
  subscribeDiagnosticToolMaterialGeneration,
} from "./diagnosticTool";
import type { DiagnosticToolGateway } from "./ports/diagnosticToolGateway";

let owner: DiagnosticToolOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
});

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

describe("diagnostic Tool material generation", () => {
  it("publishes exact Runtime replacement and final disposal boundaries", () => {
    const changed = vi.fn();
    const unsubscribe = subscribeDiagnosticToolMaterialGeneration(changed);
    const before = diagnosticToolMaterialGeneration();
    owner = DiagnosticToolOwner.install({ invoke: vi.fn() } as DiagnosticToolGateway);
    const installed = diagnosticToolMaterialGeneration();

    owner.replaceRuntimeGeneration();
    const replaced = diagnosticToolMaterialGeneration();
    owner.dispose();
    owner = undefined;

    expect([installed, replaced, diagnosticToolMaterialGeneration()]).toEqual([
      before + 1,
      before + 2,
      before + 3,
    ]);
    expect(changed).toHaveBeenCalledTimes(3);
    unsubscribe();
  });
});
