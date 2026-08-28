import { describe, expect, it } from "vitest";
import { resolveComposerModelSelection, resolveComposerRunOptions } from "./modelSelection";

function model(
  provider: string,
  id: string,
  reasoningLevels: readonly string[] = [],
  reasoningDefault?: string,
) {
  return {
    provider,
    id,
    reasoningLevelOrDefault(level?: string | null) {
      if (level && reasoningLevels.includes(level)) return level;
      if (reasoningDefault && reasoningLevels.includes(reasoningDefault)) return reasoningDefault;
      return reasoningLevels[0];
    },
  };
}

const models = [
  model("deepseek", "deepseek-chat"),
  model("deepseek", "deepseek-v4-pro", ["low", "high"], "high"),
  model("openai", "gpt-5", ["low", "medium", "high"], "medium"),
];

describe("resolveComposerModelSelection", () => {
  it("keeps an explicit model and supported effort across sessions", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "explicit", provider: "openai", model: "gpt-5", reasoningEffort: "high" },
        { provider: "deepseek", model: "deepseek-v4-pro" },
      ),
    ).toEqual({ model: models[2], reasoningEffort: "high" });
  });

  it("restores the active Session exact model and effort before a preference exists", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "session" },
        {
          provider: "deepseek",
          model: "deepseek-v4-pro",
          reasoningEffort: "low",
        },
      ),
    ).toEqual({ model: models[1], reasoningEffort: "low" });
  });

  it("does not rewrite a durable Session effort retired by a refreshed catalog", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "session" },
        {
          provider: "openai",
          model: "gpt-5",
          reasoningEffort: "retired-level",
        },
      ),
    ).toEqual({ model: models[2], reasoningEffort: "retired-level" });
  });

  it("falls back to the target model default when the prior effort is unsupported", () => {
    expect(
      resolveComposerModelSelection(
        models,
        {
          kind: "explicit",
          provider: "deepseek",
          model: "deepseek-v4-pro",
          reasoningEffort: "medium",
        },
        null,
      ),
    ).toEqual({ model: models[1], reasoningEffort: "high" });
  });

  it("restores a Session by exact provider/model when providers share a model id", () => {
    const ambiguous = [model("provider-a", "shared-model"), model("provider-b", "shared-model")];
    expect(
      resolveComposerModelSelection(
        ambiguous,
        { kind: "session" },
        {
          provider: "provider-b",
          model: "shared-model",
        },
      ),
    ).toEqual({ model: ambiguous[1], reasoningEffort: undefined });
  });

  it("waits for an active Session summary instead of racing to the catalog default", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, undefined)).toBeUndefined();
  });

  it("uses the catalog default only when no durable Session supplies one", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, null)).toEqual({
      model: models[0],
      reasoningEffort: undefined,
    });
  });
});

describe("resolveComposerRunOptions", () => {
  it("omits an override until the person makes an explicit Composer choice", () => {
    expect(resolveComposerRunOptions({ kind: "session" })).toEqual({});
  });

  it("forwards model and effort as one exact override", () => {
    expect(
      resolveComposerRunOptions({
        kind: "explicit",
        provider: "openai",
        model: "gpt-5",
        reasoningEffort: "high",
      }),
    ).toEqual({ provider: "openai", model: "gpt-5", reasoningEffort: "high" });
  });
});
