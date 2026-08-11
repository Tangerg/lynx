import { describe, expect, it } from "vitest";
import { builtinPlugins } from "./index";

describe("built-in plugin manifest", () => {
  it("declares every plugin identity exactly once", () => {
    const names = builtinPlugins.map((plugin) => plugin.name);
    expect(new Set(names).size).toBe(names.length);
  });
});
