import { describe, expect, it } from "vitest";
import { countLines, isLargePaste, LARGE_PASTE_CHARS, LARGE_PASTE_LINES } from "./largePaste";

describe("largePaste", () => {
  it("counts lines, 1 for a newline-free string", () => {
    expect(countLines("one line")).toBe(1);
    expect(countLines("a\nb\nc")).toBe(3);
    expect(countLines("trailing\n")).toBe(2); // the empty final segment counts
  });

  it("leaves a small snippet inline", () => {
    expect(isLargePaste("")).toBe(false);
    expect(isLargePaste("a short two-liner\nthat's fine")).toBe(false);
  });

  it("trips on a tall paste (line count)", () => {
    expect(isLargePaste("x\n".repeat(LARGE_PASTE_LINES))).toBe(true);
    expect(isLargePaste("x\n".repeat(LARGE_PASTE_LINES - 2))).toBe(false);
  });

  it("trips on a wide paste (a long single line)", () => {
    expect(isLargePaste("x".repeat(LARGE_PASTE_CHARS))).toBe(true);
    expect(isLargePaste("x".repeat(LARGE_PASTE_CHARS - 1))).toBe(false);
  });
});
