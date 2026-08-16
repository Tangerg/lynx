import { describe, expect, it } from "vitest";
import { DEFAULT_SLASH_HINTS, slashHintContributions } from "./slashHints";

describe("DEFAULT_SLASH_HINTS", () => {
  it("keeps the built-in hint order stable", () => {
    expect(DEFAULT_SLASH_HINTS.map((hint) => hint.cmd)).toEqual([
      "/explain",
      "/test",
      "/fix",
      "/diff",
      "/review",
      "/commit",
      "/search",
      "/plan",
    ]);
  });
});

describe("slashHintContributions", () => {
  it("carries translation keys rather than resolved copy", () => {
    expect(slashHintContributions()).toEqual([
      { cmd: "/explain", spec: { description: "slash.explain" } },
      { cmd: "/test", spec: { description: "slash.test" } },
      { cmd: "/fix", spec: { description: "slash.fix" } },
      { cmd: "/diff", spec: { description: "slash.diff" } },
      { cmd: "/review", spec: { description: "slash.review" } },
      { cmd: "/commit", spec: { description: "slash.commit" } },
      { cmd: "/search", spec: { description: "slash.search" } },
      { cmd: "/plan", spec: { description: "slash.plan" } },
    ]);
  });
});
