import type { CommandSpec } from "@/plugins/sdk";
import { describe, expect, it } from "vitest";
import { needsProviderSetup, WELCOME_SUGGESTIONS, welcomeHintCommands } from "./welcomeContent";

const command = (id: string, combo?: string): CommandSpec => ({
  id,
  label: id,
  combo,
  run: () => undefined,
});

describe("welcome content", () => {
  it("keeps the curated suggestion order", () => {
    expect(WELCOME_SUGGESTIONS.map((suggestion) => suggestion.promptKey)).toEqual([
      "welcome.suggest.refactor.prompt",
      "welcome.suggest.search.prompt",
      "welcome.suggest.review.prompt",
      "welcome.suggest.checklist.prompt",
    ]);
  });

  it("selects only hint commands that exist and expose a combo", () => {
    const hints = welcomeHintCommands([
      command("view.toggle-sidebar", "Mod+B"),
      command("command.open", "Mod+K"),
      command("chat.new"),
    ]);

    expect(hints.map((hint) => hint.id)).toEqual(["command.open", "view.toggle-sidebar"]);
  });

  it("does not show setup while providers are still loading", () => {
    expect(needsProviderSetup(undefined)).toBe(false);
  });

  it("shows setup only when every provider lacks a saved key", () => {
    expect(needsProviderSetup([{ apiKeyMasked: "" }])).toBe(true);
    expect(needsProviderSetup([{ apiKeyMasked: "" }, { apiKeyMasked: "sk-..." }])).toBe(false);
  });
});
