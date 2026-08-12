import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError, asRunId, asSegmentId, type RunEvent } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { createRunOpeningController } from "./runOpeningController";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("run opening controller", () => {
  it("suppresses a rejected opening already superseded by authoritative projection", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const onStartError = vi.fn(() => true);
    const rejected = new RpcError({
      code: -32002,
      message: "interrupt already consumed",
      data: { type: "interrupt_not_open" },
    });
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      pump: vi.fn(),
      setStartError,
    });

    controller.begin(() => Promise.reject(rejected), undefined, onStartError);

    await vi.waitFor(() => expect(onStartError).toHaveBeenCalledOnce());
    expect(error).not.toHaveBeenCalled();
    expect(setStartError).not.toHaveBeenCalled();
  });

  it("keeps a post-accept stream failure out of the opening failure channel", async () => {
    const warning = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const onResult = vi.fn();
    const onStartError = vi.fn();
    const streamFailure = new RpcError({
      code: -32002,
      message: "run finished",
      data: { type: "run_finished", detail: "the accepted run moved before reattach" },
    });
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      pump: vi.fn(async () => {
        throw streamFailure;
      }),
      setStartError,
    });

    controller.begin(
      async () => ({
        result: { runId: asRunId("run_1"), segmentId: asSegmentId("seg_1") },
        events: emptyEvents(),
      }),
      onResult,
      onStartError,
    );

    await vi.waitFor(() => expect(onResult).toHaveBeenCalledTimes(1));
    await vi.waitFor(() =>
      expect(warning).toHaveBeenCalledWith(
        "[agent] accepted run stream failed:",
        "ses_1",
        streamFailure,
      ),
    );
    expect(setStartError).not.toHaveBeenCalled();
    expect(onStartError).not.toHaveBeenCalled();
  });

  it("surfaces a transport failure through the product command channel", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const setStartError = vi.fn<(problem: AgentProblem) => void>();
    const controller = createRunOpeningController({
      sessionId: "ses_1",
      isCancelled: () => false,
      markInteracted: vi.fn(),
      setAbortController: vi.fn(),
      abortCurrent: vi.fn(),
      pump: vi.fn(),
      setStartError,
    });

    controller.begin(() => Promise.reject(new RpcTransportError("connection unavailable")));

    await vi.waitFor(() => expect(setStartError).toHaveBeenCalledWith({ code: "transport_error" }));
  });
});

async function* emptyEvents(): AsyncIterable<RunEvent> {}
