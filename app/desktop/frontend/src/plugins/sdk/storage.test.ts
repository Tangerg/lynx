import { afterEach, describe, expect, it, vi } from "vitest";
import { createStorage } from "./storage";

afterEach(() => vi.restoreAllMocks());

describe("createStorage", () => {
  it("round-trips JSON values under a namespaced key", () => {
    const s = createStorage("alpha");
    s.set("config", { x: 1, y: "hi" });
    expect(s.get("config")).toEqual({ x: 1, y: "hi" });

    // Verify the underlying key actually carries the namespace.
    expect(localStorage.getItem("scopeapp.plugin.alpha.config")).toBe(
      JSON.stringify({ x: 1, y: "hi" }),
    );
  });

  it("returns undefined for missing keys", () => {
    const s = createStorage("alpha");
    expect(s.get("missing")).toBeUndefined();
  });

  it("isolates plugins from each other", () => {
    const a = createStorage("alpha");
    const b = createStorage("beta");
    a.set("k", "from-alpha");
    b.set("k", "from-beta");
    expect(a.get("k")).toBe("from-alpha");
    expect(b.get("k")).toBe("from-beta");
  });

  it("remove deletes one key, leaves the rest", () => {
    const s = createStorage("alpha");
    s.set("a", 1);
    s.set("b", 2);
    s.remove("a");
    expect(s.get("a")).toBeUndefined();
    expect(s.get("b")).toBe(2);
  });

  it("clear wipes only this plugin's keys", () => {
    const a = createStorage("alpha");
    const b = createStorage("beta");
    a.set("k1", 1);
    a.set("k2", 2);
    b.set("k1", "kept");

    a.clear();

    expect(a.keys()).toEqual([]);
    expect(b.get("k1")).toBe("kept");
  });

  it("keys returns only this plugin's keys, stripped of the prefix", () => {
    const a = createStorage("alpha");
    const b = createStorage("beta");
    a.set("config", 1);
    a.set("history", []);
    b.set("ignored", true);

    expect(a.keys().sort()).toEqual(["config", "history"]);
  });

  it("gracefully returns non-JSON strings as-is", () => {
    const s = createStorage("alpha");
    // Bypass the typed setter to plant a raw value.
    localStorage.setItem("scopeapp.plugin.alpha.raw", "not-json");
    expect(s.get("raw")).toBe("not-json");
  });

  it("contains failures from every Storage operation", () => {
    const denied = new DOMException("denied", "SecurityError");
    const deniedStorage = {
      get length(): number {
        throw denied;
      },
      clear(): void {
        throw denied;
      },
      getItem(): string | null {
        throw denied;
      },
      key(): string | null {
        throw denied;
      },
      removeItem(): void {
        throw denied;
      },
      setItem(): void {
        throw denied;
      },
    } satisfies Storage;
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.spyOn(window, "localStorage", "get").mockReturnValue(deniedStorage);

    const storage = createStorage("alpha");
    expect(storage.get("key")).toBeUndefined();
    expect(storage.keys()).toEqual([]);
    expect(() => storage.set("key", "value")).not.toThrow();
    expect(() => storage.remove("key")).not.toThrow();
    expect(() => storage.clear()).not.toThrow();
    expect(warn).toHaveBeenCalledTimes(3);
  });
});
