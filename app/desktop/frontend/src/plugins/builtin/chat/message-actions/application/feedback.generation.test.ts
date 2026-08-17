import { afterEach, describe, expect, it, vi } from "vitest";
import {
  MessageFeedbackOwner,
  messageFeedbackRating,
  submitMessageFeedback,
  subscribeMessageFeedback,
  type MessageFeedbackGateway,
  type MessageFeedbackTarget,
} from "./feedback";

let owner: MessageFeedbackOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
});

describe("message feedback generation", () => {
  it("serializes re-rating and keeps a newer accepted rating after an older failure", async () => {
    const positive = deferred<void>();
    const createMessageFeedback = vi.fn(({ rating }: { rating: string }) =>
      rating === "positive" ? positive.promise : Promise.resolve(),
    );
    owner = MessageFeedbackOwner.install({ createMessageFeedback });
    const target = feedbackTarget("reordered");

    const older = rejected(submitMessageFeedback(target, "positive"));
    const newer = submitMessageFeedback(target, "negative");
    await vi.waitFor(() => expect(createMessageFeedback).toHaveBeenCalledTimes(1));
    expect(messageFeedbackRating(target)).toBe("negative");

    positive.reject(new Error("older feedback failed late"));
    await expect(older).resolves.toMatchObject({ message: "older feedback failed late" });
    await expect(newer).resolves.toBe("negative");
    expect(createMessageFeedback.mock.calls.map(([input]) => input.rating)).toEqual([
      "positive",
      "negative",
    ]);
    expect(messageFeedbackRating(target)).toBe("negative");
  });

  it("rolls the latest failed intent back to the last accepted rating", async () => {
    const negative = deferred<void>();
    owner = MessageFeedbackOwner.install({
      createMessageFeedback: vi.fn(({ rating }) =>
        rating === "negative" ? negative.promise : Promise.resolve(),
      ),
    });
    const target = feedbackTarget("rollback");
    await submitMessageFeedback(target, "positive");

    const failed = rejected(submitMessageFeedback(target, "negative"));
    expect(messageFeedbackRating(target)).toBe("negative");
    negative.reject(new Error("negative rejected"));

    await expect(failed).resolves.toMatchObject({ message: "negative rejected" });
    expect(messageFeedbackRating(target)).toBe("positive");
  });

  it("retires in-flight and queued commands before a Runtime replacement can publish material", async () => {
    const retired = deferred<void>();
    const createMessageFeedback = vi.fn(() => retired.promise);
    owner = MessageFeedbackOwner.install({ createMessageFeedback });
    const target = feedbackTarget("runtime_replacement");
    const listener = vi.fn();
    const unsubscribe = subscribeMessageFeedback(target, listener);

    const inFlight = rejected(submitMessageFeedback(target, "positive"));
    const queued = rejected(submitMessageFeedback(target, "negative"));
    await vi.waitFor(() => expect(createMessageFeedback).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration();

    await expect(inFlight).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    expect(messageFeedbackRating(target)).toBeUndefined();
    const notificationsAtReplacement = listener.mock.calls.length;

    retired.resolve();
    await Promise.resolve();
    expect(createMessageFeedback).toHaveBeenCalledOnce();
    expect(messageFeedbackRating(target)).toBeUndefined();
    expect(listener).toHaveBeenCalledTimes(notificationsAtReplacement);
    unsubscribe();
  });

  it("retires an admitted command immediately on final disposal", async () => {
    const pending = deferred<void>();
    owner = MessageFeedbackOwner.install({
      createMessageFeedback: vi.fn(() => pending.promise),
    });
    const command = rejected(submitMessageFeedback(feedbackTarget("disposed"), "positive"));

    owner.dispose();
    await expect(command).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });

    pending.resolve();
    await Promise.resolve();
  });

  it("replaces a Plugin Host owner without lending queued work to its successor", async () => {
    const retired = deferred<void>();
    const retiredGateway: MessageFeedbackGateway = {
      createMessageFeedback: vi.fn(() => retired.promise),
    };
    const successorGateway: MessageFeedbackGateway = {
      createMessageFeedback: vi.fn().mockResolvedValue(undefined),
    };
    const predecessor = MessageFeedbackOwner.install(retiredGateway);
    const target = feedbackTarget("host_replacement");
    const inFlight = rejected(submitMessageFeedback(target, "positive"));
    const queued = rejected(submitMessageFeedback(target, "negative"));
    await vi.waitFor(() => expect(retiredGateway.createMessageFeedback).toHaveBeenCalledOnce());

    owner = MessageFeedbackOwner.install(successorGateway);
    await expect(inFlight).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });
    expect(successorGateway.createMessageFeedback).not.toHaveBeenCalled();

    await expect(submitMessageFeedback(target, "negative")).resolves.toBe("negative");
    expect(successorGateway.createMessageFeedback).toHaveBeenCalledOnce();
    retired.resolve();
    predecessor.dispose();
  });

  it("scopes equal Item identities to their exact Session", async () => {
    const createMessageFeedback = vi.fn().mockResolvedValue(undefined);
    owner = MessageFeedbackOwner.install({ createMessageFeedback });
    const first = feedbackTarget("shared", "ses_first");
    const second = feedbackTarget("shared", "ses_second");

    await submitMessageFeedback(first, "positive");

    expect(messageFeedbackRating(first)).toBe("positive");
    expect(messageFeedbackRating(second)).toBeUndefined();
  });
});

function feedbackTarget(suffix: string, sessionId = "ses_feedback"): MessageFeedbackTarget {
  return {
    sessionId,
    messageId: `item_${suffix}`,
    runId: "run_feedback",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((settle, fail) => {
    resolve = settle;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
