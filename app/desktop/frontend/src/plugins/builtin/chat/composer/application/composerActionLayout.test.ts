import { describe, expect, it } from "vitest";
import { composerActionLayout } from "./composerActionLayout";

describe("composerActionLayout", () => {
  it("offers send when nothing is running", () => {
    expect(composerActionLayout({ running: false, hasInput: true })).toEqual({
      primary: "send",
      secondary: null,
    });
  });

  it("makes steer primary during a run, keeping stop beside it", () => {
    expect(composerActionLayout({ running: true, hasInput: true })).toEqual({
      primary: "steer",
      secondary: "stop",
    });
  });

  it("makes stop the primary target when there is nothing to steer with", () => {
    expect(composerActionLayout({ running: true, hasInput: false })).toEqual({
      primary: "stop",
      secondary: null,
    });
  });

  it("always offers stop while a run is in flight", () => {
    for (const hasInput of [true, false]) {
      const layout = composerActionLayout({ running: true, hasInput });
      expect(layout.primary === "stop" || layout.secondary === "stop").toBe(true);
    }
  });
});
