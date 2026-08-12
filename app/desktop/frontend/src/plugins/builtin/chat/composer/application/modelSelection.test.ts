import { describe, expect, it } from "vitest";
import { resolveComposerModel } from "./modelSelection";

const models = [
  { provider: "deepseek", id: "deepseek-chat", label: "Chat" },
  { provider: "deepseek", id: "deepseek-v4-pro", label: "V4 Pro" },
  { provider: "openai", id: "gpt-5", label: "GPT-5" },
];

describe("resolveComposerModel", () => {
  it("keeps an explicit process preference across sessions", () => {
    expect(
      resolveComposerModel(models, { provider: "openai", model: "gpt-5" }, "deepseek-v4-pro"),
    ).toBe(models[2]);
  });

  it("restores the active Session model before a preference exists", () => {
    expect(resolveComposerModel(models, { provider: null, model: null }, "deepseek-v4-pro")).toBe(
      models[1],
    );
  });

  it("waits for an active Session summary instead of racing to the catalog default", () => {
    expect(
      resolveComposerModel(models, { provider: null, model: null }, undefined),
    ).toBeUndefined();
  });

  it("uses the catalog default only when no durable Session supplies one", () => {
    expect(resolveComposerModel(models, { provider: null, model: null }, null)).toBe(models[0]);
  });
});
