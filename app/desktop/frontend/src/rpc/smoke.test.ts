// End-to-end smoke test — exercises the happy path of the Lyra Runtime
// Protocol v2 (docs/protocol/API.md). Uses MemoryTransport to simulate the server,
// so this runs in CI without any backend dependency.
//
// Swapping MemoryTransport for HTTPTransport validates wire compatibility
// against the real backend. The request / response / notification
// interleavings encoded here ARE the protocol contract.
//
// Coverage:
//   1. runtime.initialize          (handshake + capability negotiation)
//   2. sessions.create             (Session shape, default cwd)
//   3. runs.start                  (immediate {runId}, then RunEvent stream)
//   4. item.* + run.* StreamEvents (the v2 Item model)
//   5. run.finished{interrupt}     (R-model HITL — Run ends, resources freed)
//   6. runs.resume                 (continuation Run answering the interrupt)
//   7. run.finished{completed}     (terminates the stream)

import { afterEach, describe, expect, it } from "vitest";
import { createMemoryTransport, type MemoryTransport } from "./transports/memory";
import {
  injectRunEvent,
  injectRunFinished,
  respondSuccess,
  waitForRequest,
} from "./transports/memory.testkit";
import { createRpcClient, type RpcClient } from "./client";
import { asItemId, asRunId, asSessionId } from "./ids";
import { createMethods, type Methods } from "./methods";
import type { Item, RunEvent } from "./shapes";

function agentMessageItem(id: string, runId: string, text: string, status: Item["status"]): Item {
  return {
    id: asItemId(id),
    runId: asRunId(runId),
    status,
    createdAt: "2026-06-03T00:00:00Z",
    type: "agentMessage",
    content: [{ type: "text", text }],
  };
}

describe("smoke: v2 end-to-end happy path", () => {
  let transport: MemoryTransport;
  let client: RpcClient;
  let methods: Methods;

  afterEach(async () => {
    await client.close();
  });

  it("initialize → create → start → interrupt → resume → completed", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    // ---- Step 1: runtime.initialize ---------------------------------------
    const initPromise = methods.runtime.initialize({
      protocolVersion: "2026-06-03",
      clientInfo: { name: "smoke-test", version: "0.1" },
      capabilities: {
        events: ["run.started", "run.finished", "item.started", "item.delta", "item.completed"],
        features: {},
        interruptTypes: ["approval", "question"],
      },
    });
    const initReq = await waitForRequest(transport, "runtime.initialize");
    expect(initReq.params).toMatchObject({ protocolVersion: "2026-06-03" });
    respondSuccess(transport, initReq.id, {
      protocolVersion: "2026-06-03",
      serverInfo: { name: "lyra-runtime", version: "0.0.0", cwd: "/work", home: "/home/u" },
      capabilities: {
        protocolVersion: "2026-06-03",
        events: ["run.started", "run.finished", "item.started", "item.delta", "item.completed"],
        features: { reasoning: true, mcp: true, relocate: true, attachments: { enabled: false } },
        providers: ["anthropic"],
        limits: { maxConcurrentRuns: 8 },
      },
    });
    const init = await initPromise;
    expect(init.serverInfo.cwd).toBe("/work");
    expect(init.capabilities.providers).toEqual(["anthropic"]);

    // ---- Step 2: sessions.create ------------------------------------------
    const createPromise = methods.sessions.create({ title: "smoke" });
    const createReq = await waitForRequest(transport, "sessions.create");
    expect(createReq.params).toEqual({ title: "smoke" });
    respondSuccess(transport, createReq.id, {
      id: "ses_1",
      title: "smoke",
      status: "idle",
      model: "claude",
      cwd: "/work",
      createdAt: "2026-06-03T00:00:00Z",
      updatedAt: "2026-06-03T00:00:00Z",
      metadata: {},
    });
    const session = await createPromise;
    expect(session.id).toBe("ses_1");
    expect(session.cwd).toBe("/work");

    // ---- Step 3: runs.start -----------------------------------------------
    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "list files" }],
      mode: "agent",
    });
    const startReq = await waitForRequest(transport, "runs.start");
    expect(startReq.params).toMatchObject({ sessionId: "ses_1" });
    respondSuccess(transport, startReq.id, { runId: "run_1" });
    const { result: started, events } = await startPromise;
    expect(started.runId).toBe("run_1");

    // ---- Step 4 + 5: drive items until the interrupt ----------------------
    setTimeout(() => {
      injectRunEvent(transport, "run_1", "evt_1", {
        type: "run.started",
        run: { id: asRunId("run_1"), sessionId: asSessionId("ses_1") },
      });
      injectRunEvent(transport, "run_1", "evt_2", {
        type: "item.started",
        item: agentMessageItem("item_1", "run_1", "", "running"),
      });
      injectRunEvent(
        transport,
        "run_1",
        "evt_3",
        {
          type: "item.delta",
          itemId: asItemId("item_1"),
          delta: { type: "content", text: "Running ls…" },
        },
        false,
      );
      injectRunEvent(transport, "run_1", "evt_4", {
        type: "item.started",
        item: {
          id: asItemId("item_tool"),
          runId: asRunId("run_1"),
          status: "running",
          createdAt: "2026-06-03T00:00:00Z",
          type: "toolCall",
          tool: { name: "bash", arguments: { command: "ls" } },
        },
      });
      // R-model HITL: the run ENDS with an interrupt for the tool approval.
      injectRunFinished(transport, "run_1", "evt_5", {
        type: "interrupt",
        interrupts: [
          {
            itemId: asItemId("item_tool"),
            type: "approval",
            payload: { tool: { name: "bash", arguments: { command: "ls" } } },
          },
        ],
      });
    }, 0);

    const firstRun: RunEvent[] = [];
    for await (const ev of events) firstRun.push(ev);
    const finish = firstRun.at(-1)!;
    expect(finish.event.type).toBe("run.finished");
    expect(finish.event.type === "run.finished" && finish.event.outcome.type).toBe("interrupt");
    const interrupt =
      finish.event.type === "run.finished" && finish.event.outcome.type === "interrupt"
        ? finish.event.outcome.interrupts[0]!
        : null;
    expect(interrupt?.itemId).toBe("item_tool");

    // ---- Step 6: runs.resume (approve) ------------------------------------
    const resumePromise = methods.runs.resume({
      parentRunId: asRunId("run_1"),
      responses: [
        { itemId: asItemId("item_tool"), response: { type: "approval", decision: "approve" } },
      ],
    });
    const resumeReq = await waitForRequest(transport, "runs.resume");
    expect(resumeReq.params).toMatchObject({ parentRunId: "run_1" });
    respondSuccess(transport, resumeReq.id, { runId: "run_2" });
    const { result: resumed, events: resumeEvents } = await resumePromise;
    expect(resumed.runId).toBe("run_2");

    // ---- Step 7: continuation run completes -------------------------------
    setTimeout(() => {
      injectRunEvent(transport, "run_2", "evt_1", {
        type: "run.started",
        run: {
          id: asRunId("run_2"),
          sessionId: asSessionId("ses_1"),
          parentRunId: asRunId("run_1"),
        },
      });
      injectRunEvent(transport, "run_2", "evt_2", {
        type: "item.completed",
        item: agentMessageItem("item_2", "run_2", "Found 5 files.", "completed"),
      });
      injectRunFinished(transport, "run_2", "evt_3", {
        type: "completed",
        result: { usage: { inputTokens: 100, outputTokens: 20 }, steps: 2 },
      });
    }, 0);

    const secondRun: RunEvent[] = [];
    for await (const ev of resumeEvents) secondRun.push(ev);
    expect(secondRun.map((e) => e.event.type)).toEqual([
      "run.started",
      "item.completed",
      "run.finished",
    ]);
  });

  it("foreign-run events are filtered out (cross-run isolation)", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, { runId: "run_ours" });
    const { events } = await startPromise;

    setTimeout(() => {
      injectRunEvent(transport, "run_other", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_x", "run_other", "stolen", "completed"),
      });
      injectRunEvent(transport, "run_ours", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_ok", "run_ours", "ok", "completed"),
      });
      injectRunFinished(transport, "run_ours", "evt_2");
    }, 0);

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.runId)).toEqual(["run_ours", "run_ours"]);
  });

  it("malformed notification params are dropped (Zod boundary)", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, { runId: "run_1" });
    const { events } = await startPromise;

    setTimeout(() => {
      // Malformed: missing eventId/timestamp/durable (required by envelope schema).
      transport.inject({
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { runId: "run_1", event: { type: "item.started" } },
      });
      injectRunEvent(transport, "run_1", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_ok", "run_1", "ok", "completed"),
      });
      injectRunFinished(transport, "run_1", "evt_2");
    }, 0);

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.completed", "run.finished"]);
  });
});
