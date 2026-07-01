// Third-party namespace convention (API.md §2.5): sideloaded plugins must
// prefix the enumerable identifiers they introduce (custom event names,
// content-block kinds) with `plugin:<name>/`; first-party built-ins are
// exempt. The host warns (doesn't throw) on a violation.

import type { ContentBlockKind } from "@/plugins/sdk/types/agentView";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createHost } from "./host";
import { setPluginOrigin } from "./pluginOrigin";
import { usePluginStore } from "./registry";

beforeEach(() => usePluginStore.getState().resetForTest());
afterEach(() => vi.restoreAllMocks());

describe("third-party namespace convention (API.md §2.5)", () => {
  it("warns when a sideloaded plugin uses a bare custom event name", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    setPluginOrigin("third-bare", "sideload");
    createHost("third-bare", [], ["events"]).events.onCustom("progress", () => {});
    expect(warn).toHaveBeenCalledOnce();
    expect(String(warn.mock.calls[0]?.[0])).toContain("not namespaced");
  });

  it("accepts a namespaced custom event name from a sideloaded plugin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    setPluginOrigin("third-ns", "sideload");
    createHost("third-ns", [], ["events"]).events.onCustom("plugin:third-ns/progress", () => {});
    expect(warn).not.toHaveBeenCalled();
  });

  it("exempts first-party built-ins (bare names are fine)", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    // Not recorded as sideload → origin defaults to "builtin" (trusted).
    createHost("builtin-x", [], ["events"]).events.onCustom("progress", () => {});
    expect(warn).not.toHaveBeenCalled();
  });

  it("warns on a bare content-block kind from a sideloaded plugin", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    setPluginOrigin("third-block", "sideload");
    createHost("third-block", [], ["message"]).message.registerContentBlock(
      "fancy" as ContentBlockKind,
      (() => null) as never,
    );
    expect(warn).toHaveBeenCalledOnce();
  });
});
