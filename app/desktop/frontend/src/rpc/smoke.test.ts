// End-to-end smoke test — exercises the happy path of the Lyra Runtime
// Protocol v2 (docs/protocol/API.md). Uses MemoryTransport to simulate the server,
// so this runs in CI without any backend dependency.
//
// Swapping MemoryTransport for HTTPTransport validates wire compatibility
// against the real backend. The request / response / notification
// interleavings encoded here ARE the protocol contract.
//
// Coverage:
//   1. runtime.discover            (optional capability discovery)
//   2. sessions.create             (Session shape, default cwd)
//   3. runs.start                  (immediate {runId, segmentId}, then RunEvent stream)
//   4. item.* + run.* StreamEvents (the v2 Item model)
//   5. segment.finished{interrupt}     (R-model HITL — the segment ends, run parked)
//   6. runs.resume                 (SAME run, a NEW segment answering the interrupt)
//   7. segment.finished{completed}     (terminates the stream)

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

  it("discover → create → start → interrupt → resume → completed", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport, {
      requestMeta: () => ({
        protocolVersion: "2026-07-19",
        clientInfo: { name: "smoke-test", version: "0.1" },
        clientCapabilities: {
          events: [
            "segment.started",
            "segment.finished",
            "item.started",
            "item.delta",
            "item.completed",
          ],
          features: {},
          interruptTypes: ["approval", "question"],
        },
      }),
    });
    methods = createMethods(client);

    // ---- Step 1: runtime.discover -----------------------------------------
    const discoverPromise = methods.runtime.discover();
    const discoverReq = await waitForRequest(transport, "runtime.discover");
    expect(discoverReq.params).toMatchObject({
      _meta: {
        protocolVersion: "2026-07-19",
        clientCapabilities: { interruptTypes: ["approval", "question"] },
      },
    });
    respondSuccess(transport, discoverReq.id, {
      protocolVersion: "2026-07-19",
      serverInfo: { name: "lyra-runtime", version: "0.0.0", cwd: "/work", home: "/home/u" },
      capabilities: {
        protocolVersion: "2026-07-19",
        events: [
          "segment.started",
          "segment.finished",
          "item.started",
          "item.delta",
          "item.completed",
        ],
        features: { reasoning: true, mcp: true, relocate: true, multimodal: true },
        providers: ["anthropic"],
        streamingMethods: ["runs.start", "runs.resume", "runs.subscribe"],
        limits: { maxConcurrentRuns: 8 },
      },
    });
    const discovery = await discoverPromise;
    expect(discovery.serverInfo.cwd).toBe("/work");
    expect(discovery.capabilities.providers).toEqual(["anthropic"]);

    // ---- Step 2: sessions.create ------------------------------------------
    const createPromise = methods.sessions.create({ title: "smoke" });
    const createReq = await waitForRequest(transport, "sessions.create");
    expect(createReq.params).toMatchObject({
      title: "smoke",
      _meta: { protocolVersion: "2026-07-19" },
    });
    respondSuccess(transport, createReq.id, {
      id: "ses_1",
      title: "smoke",
      status: "idle",
      model: "claude",
      cwd: "/work",
      createdAt: "2026-06-03T00:00:00Z",
      updatedAt: "2026-06-03T00:00:00Z",
    });
    const session = await createPromise;
    expect(session.id).toBe("ses_1");
    expect(session.cwd).toBe("/work");

    // ---- Step 3: runs.start -----------------------------------------------
    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "list files" }],
    });
    const startReq = await waitForRequest(transport, "runs.start");
    expect(startReq.params).toMatchObject({ sessionId: "ses_1" });
    respondSuccess(transport, startReq.id, { runId: "run_1", segmentId: "seg_1" });
    const { result: started, events } = await startPromise;
    expect(started.runId).toBe("run_1");
    expect(started.segmentId).toBe("seg_1");

    // ---- Step 4 + 5: drive items until the interrupt ----------------------
    setTimeout(() => {
      injectRunEvent(transport, "run_1", "seg_1", "evt_1", {
        type: "segment.started",
        run: { id: asRunId("run_1"), sessionId: asSessionId("ses_1") },
      });
      injectRunEvent(transport, "run_1", "seg_1", "evt_2", {
        type: "item.started",
        item: agentMessageItem("item_1", "run_1", "", "running"),
      });
      injectRunEvent(
        transport,
        "run_1",
        "seg_1",
        "evt_3",
        {
          type: "item.delta",
          itemId: asItemId("item_1"),
          delta: { type: "content", text: "Running ls…" },
        },
        false,
      );
      injectRunEvent(transport, "run_1", "seg_1", "evt_4", {
        type: "item.started",
        item: {
          id: asItemId("item_tool"),
          runId: asRunId("run_1"),
          status: "running",
          createdAt: "2026-06-03T00:00:00Z",
          type: "toolCall",
          tool: { name: "shell", arguments: { command: "ls" } },
        },
      });
      // R-model HITL: the segment ENDS with an interrupt for the tool approval.
      injectRunFinished(transport, "run_1", "seg_1", "evt_5", {
        type: "interrupt",
        interrupts: [
          {
            itemId: asItemId("item_tool"),
            type: "approval",
            payload: { tool: { name: "shell", arguments: { command: "ls" } } },
          },
        ],
      });
    }, 0);

    const firstRun: RunEvent[] = [];
    for await (const ev of events) firstRun.push(ev);
    const finish = firstRun.at(-1)!;
    expect(finish.event.type).toBe("segment.finished");
    expect(finish.event.type === "segment.finished" && finish.event.outcome.type).toBe("interrupt");
    const interrupt =
      finish.event.type === "segment.finished" && finish.event.outcome.type === "interrupt"
        ? finish.event.outcome.interrupts[0]!
        : null;
    expect(interrupt?.itemId).toBe("item_tool");

    // ---- Step 6: runs.resume (approve) — SAME run, a NEW segment ----------
    const resumePromise = methods.runs.resume({
      runId: asRunId("run_1"),
      responses: [
        { itemId: asItemId("item_tool"), response: { type: "approval", decision: "approve" } },
      ],
    });
    const resumeReq = await waitForRequest(transport, "runs.resume");
    expect(resumeReq.params).toMatchObject({ runId: "run_1" });
    respondSuccess(transport, resumeReq.id, { runId: "run_1", segmentId: "seg_2" });
    const { result: resumed, events: resumeEvents } = await resumePromise;
    expect(resumed.runId).toBe("run_1"); // the SAME run
    expect(resumed.segmentId).toBe("seg_2"); // a NEW segment

    // ---- Step 7: continuation segment completes ---------------------------
    setTimeout(() => {
      injectRunEvent(transport, "run_1", "seg_2", "evt_1", {
        type: "segment.started",
        run: { id: asRunId("run_1"), sessionId: asSessionId("ses_1") },
      });
      injectRunEvent(transport, "run_1", "seg_2", "evt_2", {
        type: "item.completed",
        item: agentMessageItem("item_2", "run_1", "Found 5 files.", "completed"),
      });
      injectRunFinished(transport, "run_1", "seg_2", "evt_3", {
        type: "completed",
        result: { usage: { inputTokens: 100, outputTokens: 20 }, steps: 2 },
      });
    }, 0);

    const secondRun: RunEvent[] = [];
    for await (const ev of resumeEvents) secondRun.push(ev);
    expect(secondRun.map((e) => e.event.type)).toEqual([
      "segment.started",
      "item.completed",
      "segment.finished",
    ]);
  });

  it("foreign-segment events are filtered out (cross-run isolation)", async () => {
    transport = createMemoryTransport();
    client = createRpcClient(transport);
    methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(transport, "runs.start");
    respondSuccess(transport, req.id, { runId: "run_ours", segmentId: "seg_ours" });
    const { events } = await startPromise;

    setTimeout(() => {
      injectRunEvent(transport, "run_other", "seg_other", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_x", "run_other", "stolen", "completed"),
      });
      injectRunEvent(transport, "run_ours", "seg_ours", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_ok", "run_ours", "ok", "completed"),
      });
      injectRunFinished(transport, "run_ours", "seg_ours", "evt_2");
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
    respondSuccess(transport, req.id, { runId: "run_1", segmentId: "seg_1" });
    const { events } = await startPromise;

    setTimeout(() => {
      // Malformed: missing segmentId/eventId/timestamp (required by envelope schema).
      transport.inject({
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { runId: "run_1", event: { type: "item.started" } },
      });
      injectRunEvent(transport, "run_1", "seg_1", "evt_1", {
        type: "item.completed",
        item: agentMessageItem("item_ok", "run_1", "ok", "completed"),
      });
      injectRunFinished(transport, "run_1", "seg_1", "evt_2");
    }, 0);

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.completed", "segment.finished"]);
  });
});
