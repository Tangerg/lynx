import { describe, expect, it } from "vitest";
import { fmtDuration } from "./format";

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
