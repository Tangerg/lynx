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
  it("projects translation keys into hint-only slash command specs", () => {
    expect(slashHintContributions((key) => `t:${key}`)).toEqual([
      { cmd: "/explain", spec: { description: "t:slash.explain" } },
      { cmd: "/test", spec: { description: "t:slash.test" } },
      { cmd: "/fix", spec: { description: "t:slash.fix" } },
      { cmd: "/diff", spec: { description: "t:slash.diff" } },
      { cmd: "/review", spec: { description: "t:slash.review" } },
      { cmd: "/commit", spec: { description: "t:slash.commit" } },
      { cmd: "/search", spec: { description: "t:slash.search" } },
      { cmd: "/plan", spec: { description: "t:slash.plan" } },
    ]);
  });
});
