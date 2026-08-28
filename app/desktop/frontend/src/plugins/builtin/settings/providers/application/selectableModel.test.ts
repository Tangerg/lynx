import { describe, expect, it } from "vitest";

import { SelectableModel } from "./selectableModel";

describe("SelectableModel", () => {
  it("owns immutable capability collections", () => {
    const reasoningLevels = ["low", "high"];
    const inputModalities = ["text", "image"];
    const model = new SelectableModel({
      id: "gpt",
      provider: "openai",
      label: "GPT",
      reasoning: true,
      reasoningLevels,
      reasoningDefaultLevel: "high",
      inputModalities,
    });

    reasoningLevels.push("max");
    inputModalities.splice(0);

    expect(model.reasoningLevels).toEqual(["low", "high"]);
    expect(model.inputModalities).toEqual(["text", "image"]);
    expect(Object.isFrozen(model)).toBe(true);
    expect(Object.isFrozen(model.reasoningLevels)).toBe(true);
    expect(Object.isFrozen(model.inputModalities)).toBe(true);
  });

  it("owns modality admission and reasoning fallback behavior", () => {
    const model = new SelectableModel({
      id: "gpt",
      provider: "openai",
      label: "GPT",
      reasoning: true,
      reasoningLevels: ["low", "medium", "high"],
      reasoningDefaultLevel: "medium",
      inputModalities: ["text", "image"],
    });

    expect(model.acceptsInput("image")).toBe(true);
    expect(model.acceptsInput("audio")).toBe(false);
    expect(model.reasoningLevelOrDefault("high")).toBe("high");
    expect(model.reasoningLevelOrDefault("unsupported")).toBe("medium");
  });

  it("does not invent reasoning for a model that lacks it", () => {
    const model = new SelectableModel({
      id: "plain",
      provider: "example",
      label: "Plain",
      reasoningLevels: ["high"],
    });

    expect(model.acceptsReasoningLevel("high")).toBe(false);
    expect(model.reasoningLevelOrDefault()).toBeUndefined();
  });
});
