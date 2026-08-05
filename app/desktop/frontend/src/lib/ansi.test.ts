import { describe, expect, it } from "vitest";
import { hasAnsi, parseAnsi } from "./ansi";

const ESC = "\u001b";
const sgr = (params: string) => `${ESC}[${params}m`;

describe("parseAnsi", () => {
  it("leaves plain text as one span, brackets and all", () => {
    // The whole point of matching the escape byte rather than `[`: log lines are full
    // of brackets, and a looser pattern would eat them.
    expect(parseAnsi("[INFO] ready in 200ms")).toEqual([{ text: "[INFO] ready in 200ms" }]);
  });

  it("maps a colour onto a tone and closes it on reset", () => {
    expect(parseAnsi(`${sgr("31")}FAIL${sgr("0")} ok`)).toEqual([
      { text: "FAIL", tone: "negative" },
      { text: " ok" },
    ]);
  });

  it("carries weight and underline alongside the tone", () => {
    expect(parseAnsi(`${sgr("1;32")}PASS`)).toEqual([
      { text: "PASS", tone: "success", bold: true },
    ]);
    expect(parseAnsi(`${sgr("2")}dim`)).toEqual([{ text: "dim", dim: true }]);
    expect(parseAnsi(`${sgr("4")}under`)).toEqual([{ text: "under", underline: true }]);
  });

  it("reads a bright colour as its own tone, not as an unknown code", () => {
    expect(parseAnsi(`${sgr("91")}oops`)).toEqual([{ text: "oops", tone: "negative" }]);
  });

  it("skips a 256-colour selector's arguments instead of reading them as codes", () => {
    // `38;5;196` is one instruction; reading `5` and `196` as codes would leave the
    // style wrong for the rest of the line.
    expect(parseAnsi(`${sgr("38;5;196")}x${sgr("39")}y`)).toEqual([{ text: "xy" }]);
  });

  it("drops what a transcript has no cursor for", () => {
    // A progress bar redrawing itself: without dropping these, every frame it ever
    // painted would stack up in the log.
    expect(parseAnsi(`${ESC}[2K${ESC}[1Gbuilding…`)).toEqual([{ text: "building…" }]);
  });

  it("merges runs the style did not actually change", () => {
    expect(parseAnsi(`a${sgr("39")}b${sgr("39")}c`)).toEqual([{ text: "abc" }]);
  });

  it("closes only what the code closes", () => {
    expect(parseAnsi(`${sgr("1;31")}a${sgr("22")}b`)).toEqual([
      { text: "a", tone: "negative", bold: true },
      { text: "b", tone: "negative" },
    ]);
  });
});

describe("hasAnsi", () => {
  it("is false for text that merely contains brackets", () => {
    expect(hasAnsi("[warn] a[0]")).toBe(false);
    expect(hasAnsi(`${sgr("31")}x`)).toBe(true);
  });
});
