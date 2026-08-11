import { describe, expect, it } from "vitest";
import { parseGoalStartDraft } from "./goalStartDraft";

describe("parseGoalStartDraft", () => {
  it("keeps blank limits uncapped", () => {
    expect(
      parseGoalStartDraft({
        objective: "  Ship alpha  ",
        maxRuns: "",
        maxCostUsd: " ",
        maxSteps: "",
      }),
    ).toEqual({ ok: true, objective: "Ship alpha" });
  });

  it("parses positive finite limits without filling absent axes", () => {
    expect(
      parseGoalStartDraft({
        objective: "Ship alpha",
        maxRuns: "3",
        maxCostUsd: "1.25",
        maxSteps: "",
      }),
    ).toEqual({
      ok: true,
      objective: "Ship alpha",
      budget: { maxRuns: 3, maxCostUsd: 1.25 },
    });
  });

  it.each([
    ["objective", { objective: " ", maxRuns: "", maxCostUsd: "", maxSteps: "" }],
    ["maxRuns", { objective: "Ship", maxRuns: "0", maxCostUsd: "", maxSteps: "" }],
    ["maxRuns", { objective: "Ship", maxRuns: "1.5", maxCostUsd: "", maxSteps: "" }],
    ["maxCostUsd", { objective: "Ship", maxRuns: "", maxCostUsd: "-1", maxSteps: "" }],
    ["maxSteps", { objective: "Ship", maxRuns: "", maxCostUsd: "", maxSteps: "many" }],
  ])("rejects invalid %s", (field, draft) => {
    expect(parseGoalStartDraft(draft)).toEqual({ ok: false, field });
  });
});
