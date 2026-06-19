import { describe, expect, it } from "vitest";
import { activeMention } from "./useFileMentions";

describe("activeMention", () => {
  it("detects a bare @ at the start", () => {
    expect(activeMention("@", 1)).toEqual({ query: "", start: 0, end: 1 });
  });

  it("detects a mid-text mention after whitespace", () => {
    // "see @comp" — caret at end (9)
    expect(activeMention("see @comp", 9)).toEqual({ query: "comp", start: 4, end: 9 });
  });

  it("does not trigger on an @ inside a word (e.g. an email)", () => {
    expect(activeMention("user@host", 9)).toBeNull();
  });

  it("ends the mention at whitespace", () => {
    // caret sits after a space following the mention → no active mention
    expect(activeMention("@a ", 3)).toBeNull();
    // caret in a fresh token after the mention
    expect(activeMention("@a b", 4)).toBeNull();
  });

  it("returns null when there's no @ before the caret", () => {
    expect(activeMention("hello", 5)).toBeNull();
    expect(activeMention("", 0)).toBeNull();
  });
});
