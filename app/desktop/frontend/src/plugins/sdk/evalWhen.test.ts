import { describe, expect, it } from "vitest";
import { evalWhen } from "./evalWhen";

describe("evalWhen", () => {
  it("returns false for unknown identifiers", () => {
    expect(evalWhen("nope", {})).toBe(false);
  });

  it("returns true for truthy identifiers", () => {
    expect(evalWhen("flag", { flag: true })).toBe(true);
    expect(evalWhen("flag", { flag: "yes" })).toBe(true);
  });

  it("negates with !", () => {
    expect(evalWhen("!flag", { flag: false })).toBe(true);
    expect(evalWhen("!flag", { flag: true })).toBe(false);
  });

  it("compares with == and !=", () => {
    expect(evalWhen('view == "diff"', { view: "diff" })).toBe(true);
    expect(evalWhen('view == "diff"', { view: "chat" })).toBe(false);
    expect(evalWhen('view != "diff"', { view: "chat" })).toBe(true);
  });

  it("respects && precedence over ||", () => {
    // a || b && c  ===  a || (b && c)
    expect(evalWhen("a || b && c", { a: false, b: true, c: false })).toBe(false);
    expect(evalWhen("a || b && c", { a: false, b: true, c: true })).toBe(true);
    expect(evalWhen("a || b && c", { a: true, b: false, c: false })).toBe(true);
  });

  it("supports parentheses for grouping", () => {
    expect(evalWhen("(a || b) && c", { a: false, b: true, c: false })).toBe(false);
    expect(evalWhen("(a || b) && c", { a: false, b: true, c: true })).toBe(true);
  });

  it("handles realistic palette conditions", () => {
    const ctx = { mainViewActive: true, mainView: "settings", theme: "dark" };
    expect(evalWhen('mainViewActive && mainView == "settings"', ctx)).toBe(true);
    expect(evalWhen('mainViewActive && mainView == "diff"', ctx)).toBe(false);
    expect(evalWhen("!mainViewActive", ctx)).toBe(false);
    expect(evalWhen('theme == "dark" || theme == "light"', ctx)).toBe(true);
  });

  it("hides commands whose when clause has a parse error", () => {
    expect(evalWhen("&& foo", {})).toBe(false);
  });

  it("treats unclosed strings as parse error (false)", () => {
    expect(evalWhen('view == "diff', {})).toBe(false);
  });
});
