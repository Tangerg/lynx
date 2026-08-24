import { describe, expect, it } from "vitest";
import { resolveComposerModel, resolveComposerRunOptions } from "./modelSelection";

const models = [
  { provider: "deepseek", id: "deepseek-chat", label: "Chat" },
  { provider: "deepseek", id: "deepseek-v4-pro", label: "V4 Pro" },
  { provider: "openai", id: "gpt-5", label: "GPT-5" },
];

describe("resolveComposerModel", () => {
  it("keeps an explicit process preference across sessions", () => {
    expect(
      resolveComposerModel(
        models,
        { provider: "openai", model: "gpt-5" },
        {
          provider: "deepseek",
          model: "deepseek-v4-pro",
        },
      ),
    ).toBe(models[2]);
  });

  it("restores the active Session model before a preference exists", () => {
    expect(
      resolveComposerModel(
        models,
        { provider: null, model: null },
        {
          provider: "deepseek",
          model: "deepseek-v4-pro",
        },
      ),
    ).toBe(models[1]);
  });

  it("restores a Session by exact provider/model when providers share a model id", () => {
    const ambiguous = [
      { provider: "provider-a", id: "shared-model" },
      { provider: "provider-b", id: "shared-model" },
    ];

    expect(
      resolveComposerModel(
        ambiguous,
        { provider: null, model: null },
        {
          provider: "provider-b",
          model: "shared-model",
        },
      ),
    ).toBe(ambiguous[1]);
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

describe("resolveComposerRunOptions", () => {
  it("omits the pair until the person makes an explicit Composer choice", () => {
    expect(resolveComposerRunOptions({ provider: null, model: null })).toEqual({});
    expect(resolveComposerRunOptions({ provider: "openai", model: null })).toEqual({});
  });

  it("forwards an explicit provider/model pair as one override", () => {
    expect(resolveComposerRunOptions({ provider: "openai", model: "gpt-5" })).toEqual({
      provider: "openai",
      model: "gpt-5",
    });
  });
});
