import { describe, expect, it } from "vitest";
import { intraLineDiff } from "./intraLineDiff";

describe("intraLineDiff", () => {
  it("marks the differing middle, trimming common prefix + suffix", () => {
    // "foo " + bar/qux + " baz"
    expect(intraLineDiff("foo bar baz", "foo qux baz")).toEqual({ del: [4, 7], add: [4, 7] });
  });

  it("marks only the appended tail on a pure insertion", () => {
    expect(intraLineDiff("abc", "abcdef")).toEqual({ del: null, add: [3, 6] });
  });

  it("marks only the removed tail on a pure deletion", () => {
    expect(intraLineDiff("abcdef", "abc")).toEqual({ del: [3, 6], add: null });
  });

  it("returns null/null for identical lines", () => {
    expect(intraLineDiff("same", "same")).toEqual({ del: null, add: null });
  });

  it("returns null/null when the lines share no prefix or suffix", () => {
    // Wholesale change — the row tint already conveys it, no word mark.
    expect(intraLineDiff("xxx", "yyy")).toEqual({ del: null, add: null });
  });

  it("handles a shared prefix only (no common suffix)", () => {
    expect(intraLineDiff("const a = 1;", "const a = 2;")).toEqual({ del: [10, 11], add: [10, 11] });
  });
});
