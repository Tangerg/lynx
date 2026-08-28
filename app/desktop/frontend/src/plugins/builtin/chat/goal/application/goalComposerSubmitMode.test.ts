import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComposerSubmitModeContext } from "@/plugins/sdk";
import type { GoalState } from "./goalReadModel";
import { GoalComposerModeOwner } from "./goalComposerMode";
import {
  createGoalComposerSubmitMode,
  type GoalComposerSubmitDependencies,
} from "./goalComposerSubmitMode";

let owner: GoalComposerModeOwner;
let sessionId: string | null;
let composerText: string;
let goalState: GoalState;
let dependencies: GoalComposerSubmitDependencies;
let context: ComposerSubmitModeContext;

beforeEach(() => {
  owner = GoalComposerModeOwner.install();
  sessionId = "ses_a";
  composerText = "/goal ship alpha";
  goalState = { available: true, goal: null };
  dependencies = {
    activeSessionId: () => sessionId,
    composerText: () => composerText,
    goalState: () => goalState,
    runtimeAvailable: () => true,
    modelPreference: () => ({
      kind: "explicit",
      provider: "openai",
      model: "gpt-5",
      reasoningEffort: "high",
    }),
    start: vi.fn(async () => {}),
    focusComposer: vi.fn(),
    reportUnavailable: vi.fn(),
    reportUnsupportedAttachments: vi.fn(),
    reportStartError: vi.fn(),
    retired: () => false,
  };
  context = {
    rawText: composerText,
    text: composerText,
    body: composerText,
    slash: { command: "/goal", args: "ship alpha" },
    hasImages: false,
    hasPastes: false,
    accept: vi.fn(),
    clear: vi.fn(),
  };
});

afterEach(() => owner.dispose());

describe("Goal composer submit mode", () => {
  it("turns a bare /goal command into composer mode without opening a second draft", () => {
    context = {
      ...context,
      rawText: "/goal ",
      text: "/goal",
      slash: { command: "/goal", args: "" },
    };
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    expect(mode.matches(context)).toBe(true);
    mode.submit(context);

    expect(owner.snapshot()).toMatchObject({ sessionId: "ses_a", phase: "armed" });
    expect(context.clear).toHaveBeenCalledOnce();
    expect(dependencies.focusComposer).toHaveBeenCalledOnce();
    expect(dependencies.start).not.toHaveBeenCalled();
  });

  it("starts one uncapped Goal through the existing composer submit transaction", async () => {
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    mode.submit(context);

    await vi.waitFor(() =>
      expect(dependencies.start).toHaveBeenCalledWith({
        sessionId: "ses_a",
        objective: "ship alpha",
        provider: "openai",
        model: "gpt-5",
        reasoningEffort: "high",
      }),
    );
    expect(context.accept).toHaveBeenCalledOnce();
    expect(owner.snapshot().phase).toBe("inactive");
  });

  it("keeps the exact draft armed when Runtime rejects the start", async () => {
    const error = new Error("offline");
    dependencies.start = vi.fn(async () => {
      throw error;
    });
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    mode.submit(context);

    await vi.waitFor(() => expect(dependencies.reportStartError).toHaveBeenCalledWith(error));
    expect(context.accept).not.toHaveBeenCalled();
    expect(owner.snapshot().phase).toBe("armed");
  });

  it("asks before replacing a paused Goal and starts only after confirmation", async () => {
    goalState = {
      available: true,
      goal: { objective: "old objective", status: "paused" } as GoalState["goal"],
    };
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    mode.submit(context);

    expect(owner.snapshot()).toMatchObject({
      sessionId: "ses_a",
      phase: "confirming",
      replacedObjective: "old objective",
    });
    expect(dependencies.start).not.toHaveBeenCalled();

    owner.confirmReplacement("ses_a");
    await vi.waitFor(() => expect(dependencies.start).toHaveBeenCalledOnce());
    expect(context.accept).toHaveBeenCalledOnce();
  });

  it("does not consume text edited while the authoritative start is in flight", async () => {
    let finish!: () => void;
    dependencies.start = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const mode = createGoalComposerSubmitMode(owner, dependencies);
    mode.submit(context);
    composerText = "a successor draft";
    finish();

    await vi.waitFor(() => expect(owner.snapshot().phase).toBe("inactive"));
    expect(context.accept).not.toHaveBeenCalled();
  });

  it("rejects unsupported Goal attachments without clearing them", () => {
    context = { ...context, hasImages: true };
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    mode.submit(context);

    expect(dependencies.reportUnsupportedAttachments).toHaveBeenCalledOnce();
    expect(context.accept).not.toHaveBeenCalled();
    expect(context.clear).not.toHaveBeenCalled();
  });

  it("does not arm a conflicting Goal mode while one is already active", () => {
    goalState = {
      available: true,
      goal: { objective: "current objective", status: "active" } as GoalState["goal"],
    };
    const mode = createGoalComposerSubmitMode(owner, dependencies);

    mode.submit(context);

    expect(dependencies.reportUnavailable).toHaveBeenCalledOnce();
    expect(owner.snapshot().phase).toBe("inactive");
    expect(dependencies.start).not.toHaveBeenCalled();
    expect(context.accept).not.toHaveBeenCalled();
  });
});
