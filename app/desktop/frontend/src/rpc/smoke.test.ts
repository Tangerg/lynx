// End-to-end smoke test — exercises the full happy path defined in
// docs/BACKEND_REVIEW.md §14. Uses MemoryTransport to simulate the
// server, so this test runs in CI without any backend dependency.
//
// Once the real backend is up, the same scenario validates wire
// compatibility by swapping MemoryTransport for HTTPTransport (just
// change the `createMemoryTransport()` call). The interleavings of
// requests / responses / notifications encoded here ARE the protocol
// contract the frontend depends on.
//
// Coverage:
//   1. runtime.initialize     (handshake + capability negotiation)
//   2. sessions.create        (Session shape)
//   3. runs.start             (immediate Response with runId, then event stream)
//   4. notifications/run/event stream consumption (AG-UI events)
//   5. lyra.approval HITL pause → runs.approval.submit → run continues
//   6. notifications/run/closed terminates the events iterator
//   7. Final event sequence assertion

import { afterEach, describe, expect, it } from "vitest";
import { createMemoryTransport, type MemoryTransport } from "./transports/memory";
import {
  injectRunClosed,
  injectRunEvent,
  respondSuccess,
  waitForRequest,
} from "./transports/memory.testkit";
import { createRpcClient, type RpcClient } from "./client";
import { asApprovalRequestId, asMessageId, asSessionId } from "./ids";
import { createMethods, type Methods } from "./methods";
import { JSONRPC_VERSION } from "./types";

// ---------------------------------------------------------------------------
// Scenario
// ---------------------------------------------------------------------------

describe("smoke: end-to-end happy path", () => {
  let transport: MemoryTransport;
  let client: RpcClient;
  let methods: Methods;

  afterEach(async () => {
    await client.close();
  });

  it("initialize → create session → start run → events with HITL pause → close", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    // ---- Step 1: runtime.initialize ---------------------------------------
    const initPromise = methods.runtime.initialize({
      protocolVersion: "2026-05-28",
      clientInfo: { name: "smoke-test", version: "0.1" },
      capabilities: {
        events: { standard: ["TEXT_MESSAGE_START", "RUN_FINISHED"], custom: ["lyra.approval"] },
        features: { multimodal: false, markdown: true },
      },
    });
    const initReq = await waitForRequest(transport, "runtime.initialize");
    expect(initReq.params).toMatchObject({
      protocolVersion: "2026-05-28",
      clientInfo: { name: "smoke-test" },
    });
    respondSuccess(transport, initReq.id, {
      protocolVersion: "2026-05-28",
      serverInfo: { name: "lyra-core", version: "0.8.1" },
      capabilities: {
        events: {
          standard: [
            "TEXT_MESSAGE_START",
            "TEXT_MESSAGE_CONTENT",
            "TEXT_MESSAGE_END",
            "RUN_STARTED",
            "RUN_FINISHED",
            "STEP_STARTED",
            "STEP_FINISHED",
          ],
          custom: ["lyra.approval", "lyra.approval-result"],
        },
        features: {
          multimodal: false,
          reasoning: false,
          checkpoints: false,
          interrupts: false,
          background: false,
          subagents: false,
          skills: false,
          mcp: true,
          sessionExport: false,
          attachments: { enabled: false },
        },
        providers: ["openai", "anthropic"],
        limits: {},
      },
    });
    const init = await initPromise;
    expect(init.serverInfo.name).toBe("lyra-core");
    expect(init.protocolVersion).toBe("2026-05-28");
    expect(init.capabilities.providers).toEqual(["openai", "anthropic"]);

    // ---- Step 2: sessions.create ------------------------------------------
    const createPromise = methods.sessions.create({ title: "smoke" });
    const createReq = await waitForRequest(transport, "sessions.create");
    expect(createReq.params).toEqual({ title: "smoke" });
    respondSuccess(transport, createReq.id, {
      id: "s1",
      title: "smoke",
      status: "idle",
      model: "gpt-4",
      createdAt: "2026-05-28T00:00:00Z",
      updatedAt: "2026-05-28T00:00:00Z",
      metadata: {},
    });
    const session = await createPromise;
    expect(session.id).toBe("s1");
    expect(session.status).toBe("idle");

    // ---- Step 3: runs.start (immediate Response + event stream) -----------
    const startPromise = methods.runs.start({
      sessionId: asSessionId("s1"),
      messages: [
        {
          id: asMessageId("u1"),
          sessionId: asSessionId("s1"),
          role: "user",
          content: "list files in cwd",
          createdAt: "2026-05-28T00:00:01Z",
        },
      ],
    });
    const startReq = await waitForRequest(transport, "runs.start");
    expect(startReq.params).toMatchObject({ sessionId: "s1" });
    respondSuccess(transport, startReq.id, { runId: "r1" });
    const { result: runResult, events } = await startPromise;
    expect(runResult.runId).toBe("r1");

    // ---- Step 4 + 5: drive events until HITL pause ------------------------
    // Inject the first batch: RUN_STARTED → STEP_STARTED → TEXT_MESSAGE_START
    // → TEXT_MESSAGE_CONTENT (partial) → CUSTOM lyra.approval
    setTimeout(() => {
      injectRunEvent(transport, "r1", "1", { type: "RUN_STARTED", runId: "r1" });
      injectRunEvent(transport, "r1", "2", { type: "STEP_STARTED", stepName: "deciding" });
      injectRunEvent(transport, "r1", "3", { type: "TEXT_MESSAGE_START", messageId: "m1" });
      injectRunEvent(transport, "r1", "4", {
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "m1",
        delta: "I need to run `ls`. ",
      });
      injectRunEvent(transport, "r1", "5", {
        type: "CUSTOM",
        name: "lyra.approval",
        value: {
          requestId: "approval-1",
          parentMessageId: "m1",
          text: "Run shell command?",
          command: "ls",
          reason: "list files in cwd",
        },
      });
    }, 0);

    // Consume events manually so we can pause mid-stream without
    // calling `iterator.return()` (which would close the channel).
    const collected: Array<Record<string, unknown>> = [];
    const iter = events[Symbol.asyncIterator]();
    let approvalRequestId: string | null = null;

    for (let i = 0; i < 5; i++) {
      const next = await iter.next();
      if (next.done) throw new Error("stream ended before approval");
      collected.push(next.value as Record<string, unknown>);
      const ev = next.value as { type: string; name?: string; value?: { requestId: string } };
      if (ev.type === "CUSTOM" && ev.name === "lyra.approval") {
        approvalRequestId = ev.value?.requestId ?? null;
        break;
      }
    }
    expect(approvalRequestId).toBe("approval-1");

    // ---- Step 6: runs.approval.submit (user approves) ---------------------
    const approvePromise = methods.runs.approval.submit({
      requestId: asApprovalRequestId("approval-1"),
      decision: "approve",
    });
    const approveReq = await waitForRequest(transport, "runs.approval.submit");
    expect(approveReq.params).toEqual({
      requestId: "approval-1",
      decision: "approve",
    });
    respondSuccess(transport, approveReq.id, null);
    await approvePromise;

    // ---- Step 7: continue events through to RUN_FINISHED + run/closed -----
    setTimeout(() => {
      injectRunEvent(transport, "r1", "6", {
        type: "CUSTOM",
        name: "lyra.approval-result",
        value: { requestId: "approval-1", decision: "approve" },
      });
      injectRunEvent(transport, "r1", "7", {
        type: "TEXT_MESSAGE_CONTENT",
        messageId: "m1",
        delta: "Done. Found 5 files.",
      });
      injectRunEvent(transport, "r1", "8", { type: "TEXT_MESSAGE_END", messageId: "m1" });
      injectRunEvent(transport, "r1", "9", { type: "STEP_FINISHED", stepName: "deciding" });
      injectRunEvent(transport, "r1", "10", { type: "RUN_FINISHED", runId: "r1" });
      injectRunClosed(transport, "r1");
    }, 0);

    while (true) {
      const next = await iter.next();
      if (next.done) break;
      collected.push(next.value as Record<string, unknown>);
    }

    // ---- Final assertions -------------------------------------------------
    expect(collected).toHaveLength(10);
    expect(collected[0]).toMatchObject({ type: "RUN_STARTED" });
    expect(collected[4]).toMatchObject({ type: "CUSTOM", name: "lyra.approval" });
    expect(collected[5]).toMatchObject({ type: "CUSTOM", name: "lyra.approval-result" });
    expect(collected[9]).toMatchObject({ type: "RUN_FINISHED" });
  });

  it("foreign-run events are filtered out (cross-run isolation)", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    // Skip the handshake — capability gating is asserted in the main
    // scenario; here we focus on the per-runId filter contract.
    const startPromise = methods.runs.start({
      sessionId: asSessionId("s1"),
      messages: [],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, { runId: "ours" });
    const { events } = await startPromise;

    setTimeout(() => {
      // Foreign run — must not leak in.
      injectRunEvent(transport, "other", "1", { type: "STOLEN_EVENT" });
      // Our run.
      injectRunEvent(transport, "ours", "1", { type: "OK_EVENT" });
      injectRunClosed(transport, "ours");
    }, 0);

    const collected: Array<{ type?: string }> = [];
    for await (const ev of events) collected.push(ev as { type?: string });
    expect(collected).toHaveLength(1);
    expect(collected[0]!.type).toBe("OK_EVENT");
  });

  it("malformed notification params are dropped (Zod boundary)", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("s1"),
      messages: [],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, { runId: "r1" });
    const { events } = await startPromise;

    setTimeout(() => {
      // Malformed: missing eventId (required by RunEventParamsSchema).
      transport.inject({
        jsonrpc: JSONRPC_VERSION,
        method: "notifications/run/event",
        params: { runId: "r1", event: { type: "GARBAGE" } },
      });
      // Well-formed follow-up — must still arrive.
      injectRunEvent(transport, "r1", "1", { type: "OK_EVENT" });
      injectRunClosed(transport, "r1");
    }, 0);

    const collected: Array<{ type?: string }> = [];
    for await (const ev of events) collected.push(ev as { type?: string });
    expect(collected).toHaveLength(1);
    expect(collected[0]!.type).toBe("OK_EVENT");
  });
});
