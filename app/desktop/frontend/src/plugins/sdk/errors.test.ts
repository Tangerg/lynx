import { describe, expect, it } from "vitest";
import { reportPluginError, usePluginErrorStore } from "./errors";

describe("usePluginErrorStore", () => {
  it("push assigns a monotonically increasing id + timestamp", () => {
    const { push } = usePluginErrorStore.getState();
    push({ plugin: "p", source: "setup", message: "first" });
    push({ plugin: "p", source: "setup", message: "second" });

    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(2);
    expect(log[0]!.id).toBe(1);
    expect(log[1]!.id).toBe(2);
    expect(log[0]!.timestamp).toBeLessThanOrEqual(log[1]!.timestamp);
  });

  it("clearFor removes only the given plugin's entries", () => {
    const { push, clearFor } = usePluginErrorStore.getState();
    push({ plugin: "a", source: "setup", message: "1" });
    push({ plugin: "b", source: "setup", message: "2" });
    push({ plugin: "a", source: "render", message: "3" });

    clearFor("a");

    const remaining = usePluginErrorStore.getState().log;
    expect(remaining).toHaveLength(1);
    expect(remaining[0]!.plugin).toBe("b");
  });

  it("clearAll empties the log", () => {
    const { push, clearAll } = usePluginErrorStore.getState();
    push({ plugin: "a", source: "setup", message: "x" });
    push({ plugin: "b", source: "setup", message: "y" });

    clearAll();

    expect(usePluginErrorStore.getState().log).toEqual([]);
  });
});

describe("reportPluginError", () => {
  it("captures Error.message", () => {
    reportPluginError("p", "events", new Error("boom"));
    const e = usePluginErrorStore.getState().log[0]!;
    expect(e.plugin).toBe("p");
    expect(e.source).toBe("events");
    expect(e.message).toBe("boom");
  });

  it("stringifies non-Error values", () => {
    reportPluginError("p", "command", "raw-string");
    expect(usePluginErrorStore.getState().log[0]!.message).toBe("raw-string");
  });

  it("forwards the optional detail", () => {
    reportPluginError("p", "render", new Error("x"), "at Foo.render");
    expect(usePluginErrorStore.getState().log[0]!.detail).toBe("at Foo.render");
  });
});
