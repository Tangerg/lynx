import type { Disposable } from "./types";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineExtensionPoint } from "./defineExtensionPoint";
import { createHost } from "./host";
import { usePluginStore } from "./registry";
import { lookupExtensionPoint } from "./selectors";

// Two demo points exercising both keying strategies.
interface Format {
  id: string;
  label: string;
  order?: number;
}
const FORMAT = defineExtensionPoint<Format>({ id: "test.format", keying: "single" });
const HANDLER = defineExtensionPoint<{ run: () => void }>({ id: "test.handler", keying: "multi" });
// A point keyed by something other than `id`.
const ICON = defineExtensionPoint<{ glyph: string; fn: string }>({
  id: "test.icon",
  keying: "single",
  keyOf: (i) => i.fn,
});

beforeEach(() => usePluginStore.getState().resetForTest());
afterEach(() => vi.restoreAllMocks());

describe("extension point substrate", () => {
  it("single point: contribute + lookup round-trips, sorted by order", () => {
    const sink: Disposable[] = [];
    const host = createHost("alpha", sink);
    host.extensions.contribute(FORMAT, { id: "json", label: "JSON", order: 20 });
    host.extensions.contribute(FORMAT, { id: "md", label: "Markdown", order: 10 });

    expect(lookupExtensionPoint(FORMAT).map((f) => f.id)).toEqual(["md", "json"]);
  });

  it("single point: same key overrides + warns across plugins", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    createHost("alpha", []).extensions.contribute(FORMAT, { id: "json", label: "A" });
    createHost("beta", []).extensions.contribute(FORMAT, { id: "json", label: "B" });

    expect(warn).toHaveBeenCalledOnce();
    expect(warn.mock.calls[0]![0]).toMatch(
      /beta overrides contribution "json" on point "test.format"/,
    );
    expect(lookupExtensionPoint(FORMAT)).toHaveLength(1);
    expect(lookupExtensionPoint(FORMAT)[0]!.label).toBe("B");
  });

  it("single point: custom keyOf dedupes by that field", () => {
    const host = createHost("alpha", []);
    host.extensions.contribute(ICON, { fn: "bash", glyph: "terminal" });
    host.extensions.contribute(ICON, { fn: "bash", glyph: "shell" }); // same fn → override
    host.extensions.contribute(ICON, { fn: "grep", glyph: "search" });

    const glyphs = lookupExtensionPoint(ICON)
      .map((i) => `${i.fn}:${i.glyph}`)
      .sort();
    expect(glyphs).toEqual(["bash:shell", "grep:search"]);
  });

  it("multi point: every contribution coexists (no override)", () => {
    const a = createHost("alpha", []);
    const b = createHost("beta", []);
    a.extensions.contribute(HANDLER, { run: () => {} });
    a.extensions.contribute(HANDLER, { run: () => {} });
    b.extensions.contribute(HANDLER, { run: () => {} });

    expect(lookupExtensionPoint(HANDLER)).toHaveLength(3);
  });

  it("dispose removes the contribution (single)", () => {
    const host = createHost("alpha", []);
    const d = host.extensions.contribute(FORMAT, { id: "json", label: "JSON" });
    expect(lookupExtensionPoint(FORMAT)).toHaveLength(1);
    d.dispose();
    expect(lookupExtensionPoint(FORMAT)).toHaveLength(0);
  });

  it("dispose removes the contribution (multi)", () => {
    const host = createHost("alpha", []);
    const d1 = host.extensions.contribute(HANDLER, { run: () => {} });
    host.extensions.contribute(HANDLER, { run: () => {} });
    expect(lookupExtensionPoint(HANDLER)).toHaveLength(2);
    d1.dispose();
    expect(lookupExtensionPoint(HANDLER)).toHaveLength(1);
  });

  it("points are isolated from each other", () => {
    const host = createHost("alpha", []);
    host.extensions.contribute(FORMAT, { id: "json", label: "JSON" });
    host.extensions.contribute(HANDLER, { run: () => {} });

    expect(lookupExtensionPoint(FORMAT)).toHaveLength(1);
    expect(lookupExtensionPoint(HANDLER)).toHaveLength(1);
  });

  it("capabilities gate: extensions must be declared when restricting", () => {
    const restricted = createHost("alpha", [], ["commands"]); // no "extensions"
    expect(() => restricted.extensions.contribute(FORMAT, { id: "x", label: "X" })).toThrow(
      /not in this plugin's declared capabilities/,
    );
  });
});
