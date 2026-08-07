import { describe, expect, it, vi } from "vitest";
import { fmtCost, fmtDuration, fmtTokens } from "./format";

const locale = vi.hoisted(() => ({ current: "en" }));
vi.mock("./i18n", () => ({ activeLocale: () => locale.current }));

describe("fmtDuration", () => {
  it("keeps one decimal under ten seconds and drops it above", () => {
    expect(fmtDuration(412)).toBe("0.4s");
    expect(fmtDuration(9840)).toBe("9.8s");
    expect(fmtDuration(42_300)).toBe("42s");
  });

  it("never reads 60s", () => {
    expect(fmtDuration(59_600)).toBe("1m 00s");
  });

  it("pads the seconds column so minute readings align", () => {
    expect(fmtDuration(246_000)).toBe("4m 06s");
    expect(fmtDuration(738_000)).toBe("12m 18s");
  });
});

describe("fmtTokens", () => {
  it("stays exact below a thousand and compacts above", () => {
    expect(fmtTokens(0)).toBe("0");
    expect(fmtTokens(999)).toBe("999");
    expect(fmtTokens(1234)).toBe("1.2k");
    expect(fmtTokens(1_200_000)).toBe("1.2M");
  });

  it("drops the decimal on whole thousands but keeps it on millions", () => {
    expect(fmtTokens(12_000)).toBe("12k");
    expect(fmtTokens(1_000_000)).toBe("1.0M");
  });
});

describe("fmtCost", () => {
  it("shows sub-cent spend rather than rounding it to nothing", () => {
    expect(fmtCost(0.0042)).toBe("$0.0042");
    expect(fmtCost(0)).toBe("$0.00");
    expect(fmtCost(1.5)).toBe("$1.50");
  });
});

// The reason these route through Intl at all. Five of the eight shipped locales write a
// comma there, and "1,2k" against "1.2k" is the difference between one and a fifth and a
// thousand-odd.
describe("in a locale that writes decimals with a comma", () => {
  it("follows the locale for the number and leaves the unit alone", () => {
    locale.current = "de";
    try {
      expect(fmtTokens(1234)).toBe("1,2k");
      expect(fmtTokens(1_200_000)).toBe("1,2M");
      expect(fmtCost(1.5)).toBe("$1,50");
      expect(fmtDuration(412)).toBe("0,4s");
      // No grouping, so a column's width does not depend on the language.
      expect(fmtTokens(999)).toBe("999");
    } finally {
      locale.current = "en";
    }
  });
});
