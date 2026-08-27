import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { submitMessageFeedback } from "../application/feedback";
import { installRuntimeFeedbackGateway } from "./runtimeFeedback";

let dispose: (() => void) | undefined;

afterEach(() => {
  dispose?.();
  dispose = undefined;
  resetContainer();
});

describe("runtimeFeedbackGateway", () => {
  it("captures the exact client and submits the Session identity admitted with the message", async () => {
    const create = vi.fn().mockResolvedValue(undefined);
    const successorCreate = vi.fn().mockResolvedValue(undefined);
    setContainer({ client: () => ({ feedback: { create } }) as unknown as ScopeAppClient });
    const installation = installRuntimeFeedbackGateway();
    dispose = installation.dispose;
    setContainer({
      client: () => ({ feedback: { create: successorCreate } }) as unknown as ScopeAppClient,
    });

    await submitMessageFeedback(
      {
        sessionId: "ses_original",
        messageId: "item_feedback",
        runId: "run_feedback",
      },
      "positive",
    );

    expect(create).toHaveBeenCalledWith({
      sessionId: "ses_original",
      runId: "run_feedback",
      itemId: "item_feedback",
      rating: "positive",
    });
    expect(successorCreate).not.toHaveBeenCalled();
  });
});
