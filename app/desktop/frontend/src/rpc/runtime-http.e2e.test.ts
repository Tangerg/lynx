// @vitest-environment node

import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import {
  createServer as createHttpServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { createLyraClient, type LyraClient } from "./sdk";
import { RpcError } from "./errors";
import { asRunId, asSegmentId, asSessionId } from "./ids";
import { errorType } from "./types";
import { createSidecarClient } from "./sidecar";
import { createHttpTransport } from "./transports/http";
import { PROTOCOL_VERSION, type RunEvent, type RuntimeEvent } from "./wire.generated";

const execFileAsync = promisify(execFile);
const runtimeDirectory = resolve(process.cwd(), "../../runtime");
const managedSkillName = "runtime-http-e2e";

async function unusedLoopbackPort(): Promise<number> {
  const server = createNetServer();
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (address === null || typeof address === "string") throw new Error("failed to allocate port");
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
  return address.port;
}

interface FakeChatRequest {
  messages?: Array<{ role?: string; content?: unknown }>;
  model?: string;
  stream?: boolean;
  tools?: Array<{ function?: { name?: string } }>;
}

interface FakeToolCall {
  arguments: string;
  name: string;
}

interface Deferred {
  promise: Promise<void>;
  resolve: () => void;
}

interface ProviderGate {
  arrived: Deferred;
  claimed: boolean;
  closed: Deferred;
  marker: string;
  release: Deferred;
}

function deferred(): Deferred {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function createProviderGate(marker: string): ProviderGate {
  return {
    arrived: deferred(),
    claimed: false,
    closed: deferred(),
    marker,
    release: deferred(),
  };
}

async function within<T>(promise: Promise<T>, detail: string): Promise<T> {
  const deadline = AbortSignal.timeout(5_000);
  const timeout = new Promise<never>((_resolve, reject) => {
    deadline.addEventListener("abort", () => reject(new Error(`timed out waiting for ${detail}`)), {
      once: true,
    });
  });
  return Promise.race([promise, timeout]);
}

async function requestJson(request: IncomingMessage): Promise<FakeChatRequest> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as FakeChatRequest;
}

function scriptedReply(body: FakeChatRequest): { text?: string; tool?: FakeToolCall } {
  const transcript = JSON.stringify(body.messages ?? []);
  const availableTools = new Set(
    (body.tools ?? []).flatMap((tool) => (tool.function?.name ? [tool.function.name] : [])),
  );
  const toolResultCount = (body.messages ?? []).filter((message) => message.role === "tool").length;
  const hasToolResult = toolResultCount > 0;

  if (transcript.includes("E2E_HITL") && availableTools.has("ask_user") && !hasToolResult) {
    return {
      tool: {
        name: "ask_user",
        arguments: JSON.stringify({
          questions: [{ question: "Continue the HTTP lifecycle check?", header: "Continue" }],
        }),
      },
    };
  }
  if (transcript.includes("E2E_PLAN") && availableTools.has("set_plan") && !hasToolResult) {
    return {
      tool: {
        name: "set_plan",
        arguments: JSON.stringify({
          steps: [
            { description: "Inspect the runtime contract", status: "completed" },
            { description: "Verify frontend reconciliation", status: "in_progress" },
          ],
        }),
      },
    };
  }
  if (transcript.includes("E2E_APPROVAL") && availableTools.has("shell") && !hasToolResult) {
    return {
      tool: {
        name: "shell",
        arguments: JSON.stringify({
          command: "printf approval-ok",
          description: "Print the approval marker",
        }),
      },
    };
  }
  if (
    transcript.includes("E2E_FILE_WRITE") &&
    availableTools.has("read") &&
    availableTools.has("apply_patch") &&
    toolResultCount === 0
  ) {
    return {
      tool: {
        name: "read",
        arguments: JSON.stringify({ path: "agent-write.txt" }),
      },
    };
  }
  if (
    transcript.includes("E2E_FILE_WRITE") &&
    availableTools.has("apply_patch") &&
    toolResultCount === 1
  ) {
    return {
      tool: {
        name: "apply_patch",
        arguments: JSON.stringify({
          patch: [
            "--- a/agent-write.txt",
            "+++ b/agent-write.txt",
            "@@ -1 +1 @@",
            "-before",
            "+after",
            "",
          ].join("\n"),
        }),
      },
    };
  }
  return { text: "HTTP runtime lifecycle complete." };
}

function writeChatCompletion(
  response: ServerResponse,
  body: FakeChatRequest,
  sequence: number,
): void {
  const id = `chatcmpl-e2e-${sequence}`;
  const model = body.model ?? "e2e-model";
  const reply = scriptedReply(body);
  const finishReason = reply.tool ? "tool_calls" : "stop";
  const message = reply.tool
    ? {
        role: "assistant",
        content: "",
        tool_calls: [
          {
            id: `call-e2e-${sequence}`,
            type: "function",
            function: { name: reply.tool.name, arguments: reply.tool.arguments },
          },
        ],
      }
    : { role: "assistant", content: reply.text };

  if (!body.stream) {
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(
      JSON.stringify({
        id,
        object: "chat.completion",
        model,
        choices: [{ index: 0, message, finish_reason: finishReason }],
        usage: { prompt_tokens: 8, completion_tokens: 4, total_tokens: 12 },
      }),
    );
    return;
  }

  response.writeHead(200, { "Content-Type": "text/event-stream" });
  const chunks = reply.tool
    ? [
        { choices: [{ index: 0, delta: { role: "assistant" }, finish_reason: null }] },
        {
          choices: [
            {
              index: 0,
              delta: {
                tool_calls: [
                  {
                    index: 0,
                    id: `call-e2e-${sequence}`,
                    type: "function",
                    function: { name: reply.tool.name, arguments: reply.tool.arguments },
                  },
                ],
              },
              finish_reason: null,
            },
          ],
        },
        { choices: [{ index: 0, delta: {}, finish_reason: finishReason }] },
      ]
    : [
        {
          choices: [
            {
              index: 0,
              delta: { role: "assistant", content: reply.text },
              finish_reason: finishReason,
            },
          ],
        },
      ];
  for (const chunk of chunks) {
    response.write(
      `data: ${JSON.stringify({ id, object: "chat.completion.chunk", model, ...chunk })}\n\n`,
    );
  }
  response.write(
    `data: ${JSON.stringify({
      id,
      object: "chat.completion.chunk",
      model,
      choices: [],
      usage: { prompt_tokens: 8, completion_tokens: 4, total_tokens: 12 },
    })}\n\n`,
  );
  response.end("data: [DONE]\n\n");
}

async function waitUntilReady(baseUrl: string, processError: () => string): Promise<void> {
  for (let attempt = 0; attempt < 200; attempt++) {
    try {
      const response = await fetch(`${baseUrl}/v2/health/ready`);
      if (response.ok) return;
    } catch {
      // The process has not bound its socket yet.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`runtime did not become ready: ${processError()}`);
}

async function nextRuntimeEvent(
  iterator: AsyncIterator<RuntimeEvent>,
  type: RuntimeEvent["type"],
): Promise<RuntimeEvent> {
  const deadline = AbortSignal.timeout(5_000);
  const timeout = new Promise<never>((_resolve, reject) => {
    deadline.addEventListener("abort", () => reject(new Error(`timed out waiting for ${type}`)), {
      once: true,
    });
  });
  while (!deadline.aborted) {
    const result = await Promise.race([iterator.next(), timeout]);
    if (result.done) throw new Error(`runtime event stream ended before ${type}`);
    if (result.value.type === type) return result.value;
  }
  throw new Error(`timed out waiting for ${type}`);
}

async function collectSessionRuntimeEvents(
  iterator: AsyncIterator<RuntimeEvent>,
  types: readonly RuntimeEvent["type"][],
  sessionId: string,
): Promise<RuntimeEvent[]> {
  const pending = new Set<RuntimeEvent["type"]>(types);
  const collected: RuntimeEvent[] = [];
  const deadline = AbortSignal.timeout(5_000);
  const timeout = new Promise<never>((_resolve, reject) => {
    deadline.addEventListener(
      "abort",
      () => reject(new Error(`timed out waiting for ${[...pending].join(", ")}`)),
      { once: true },
    );
  });
  while (pending.size > 0 && !deadline.aborted) {
    const result = await Promise.race([iterator.next(), timeout]);
    if (result.done) throw new Error("runtime event stream ended before aggregate reconciliation");
    const event = result.value;
    if (pending.has(event.type) && "sessionIds" in event && event.sessionIds?.includes(sessionId)) {
      pending.delete(event.type);
      collected.push(event);
    }
  }
  if (pending.size > 0) throw new Error(`timed out waiting for ${[...pending].join(", ")}`);
  return collected;
}

async function collectRunEvents(events: AsyncIterable<RunEvent>): Promise<RunEvent[]> {
  const collected: RunEvent[] = [];
  for await (const event of events) collected.push(event);
  return collected;
}

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK", () => {
  let environmentRoot = "";
  let root = "";
  let baseUrl = "";
  let providerBaseUrl = "";
  let runtime: ReturnType<typeof import("node:child_process").spawn> | undefined;
  let provider: ReturnType<typeof createHttpServer> | undefined;
  let providerGate: ProviderGate | undefined;
  let client: LyraClient | undefined;
  let processOutput = "";

  beforeAll(async () => {
    environmentRoot = await mkdtemp(join(tmpdir(), "lyra-runtime-e2e-"));
    root = join(environmentRoot, "workspace");
    const runtimeHome = join(environmentRoot, "home");
    const runtimeData = join(environmentRoot, "runtime-data");
    await Promise.all([mkdir(root, { recursive: true }), mkdir(runtimeHome, { recursive: true })]);
    const managedSkillDirectory = join(runtimeData, "skills", managedSkillName);
    await mkdir(managedSkillDirectory, { recursive: true });
    await writeFile(
      join(managedSkillDirectory, "SKILL.md"),
      `---\nname: ${managedSkillName}\ndescription: Exercise the real Runtime skill lifecycle.\n---\n\nRun the Runtime HTTP E2E suite.\n`,
    );
    let providerCalls = 0;
    provider = createHttpServer(async (request, response) => {
      try {
        if (request.method === "GET" && request.url?.endsWith("/models")) {
          response.writeHead(200, { "Content-Type": "application/json" });
          response.end(
            JSON.stringify({ object: "list", data: [{ id: "e2e-model", object: "model" }] }),
          );
          return;
        }
        if (request.method !== "POST" || !request.url?.endsWith("/chat/completions")) {
          response.writeHead(404).end();
          return;
        }
        const body = await requestJson(request);
        const gate = providerGate;
        if (
          gate !== undefined &&
          !gate.claimed &&
          JSON.stringify(body.messages ?? []).includes(gate.marker)
        ) {
          gate.claimed = true;
          response.once("close", gate.closed.resolve);
          gate.arrived.resolve();
          await gate.release.promise;
          if (response.destroyed) return;
        }
        writeChatCompletion(response, body, ++providerCalls);
      } catch (error) {
        response.writeHead(500, { "Content-Type": "application/json" });
        response.end(JSON.stringify({ error: { message: String(error) } }));
      }
    });
    await new Promise<void>((resolve, reject) => {
      provider?.once("error", reject);
      provider?.listen(0, "127.0.0.1", resolve);
    });
    const providerAddress = provider.address();
    if (providerAddress === null || typeof providerAddress === "string") {
      throw new Error("failed to start fake model provider");
    }
    providerBaseUrl = `http://127.0.0.1:${providerAddress.port}`;

    const executable = join(environmentRoot, "lyra-e2e");
    await execFileAsync("go", ["build", "-o", executable, "./cmd/lyra"], {
      cwd: runtimeDirectory,
    });

    const port = await unusedLoopbackPort();
    baseUrl = `http://127.0.0.1:${port}`;
    const { spawn } = await import("node:child_process");
    runtime = spawn(executable, [], {
      cwd: root,
      env: {
        ...process.env,
        HOME: runtimeHome,
        LYRA_HOME: runtimeData,
        LYRA_PROVIDER: "openai-compatible",
        LYRA_MODEL: "e2e-model",
        LYRA_APIKEY: "e2e-placeholder-key",
        LYRA_BASEURL: providerBaseUrl,
        OPENAI_COMPATIBLE_API_KEY: "e2e-placeholder-key",
        LYRA_SERVER_LISTEN: `127.0.0.1:${port}`,
        LYRA_SERVER_NOLOCALTOKEN: "true",
        LYRA_MCP_SERVERS: "",
        LYRA_A2A_AGENTS: "",
        OTEL_SDK_DISABLED: "true",
      },
      stdio: ["ignore", "pipe", "pipe"],
    });
    runtime.stdout?.on("data", (chunk: Buffer) => {
      processOutput += chunk.toString();
    });
    runtime.stderr?.on("data", (chunk: Buffer) => {
      processOutput += chunk.toString();
    });
    await waitUntilReady(baseUrl, () => processOutput);

    client = createLyraClient(createHttpTransport({ baseUrl }), {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "runtime-http-e2e", version: "1" },
        clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
      }),
    });
  }, 60_000);

  afterAll(async () => {
    await client?.close();
    if (runtime?.exitCode === null) {
      runtime.kill("SIGTERM");
      await Promise.race([
        new Promise<void>((resolve) => runtime?.once("exit", () => resolve())),
        new Promise<void>((resolve) => setTimeout(resolve, 5_000)),
      ]);
      if (runtime.exitCode === null) runtime.kill("SIGKILL");
    }
    if (provider?.listening) {
      await new Promise<void>((resolve, reject) =>
        provider?.close((error) => (error ? reject(error) : resolve())),
      );
    }
    if (environmentRoot) await rm(environmentRoot, { recursive: true, force: true });
  }, 10_000);

  it("validates discovery, streaming notifications and session lifecycle", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const sidecar = createSidecarClient({ baseUrl });
    await expect(sidecar.liveness()).resolves.toEqual({ status: "ok" });
    await expect(sidecar.readiness()).resolves.toMatchObject({ status: "ok" });
    await expect(sidecar.info()).resolves.toMatchObject({
      protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
      transport: "http",
    });

    const discovery = await client.runtime.discover();
    expect(discovery.protocol).toEqual({
      current: PROTOCOL_VERSION,
      minSupported: PROTOCOL_VERSION,
    });
    expect(discovery.capabilities.streamingMethods).toContain("runtime.subscribe");

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    const created = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP contract e2e",
    });
    const sessionId = asSessionId(created.id);
    expect(created.revision).toBe(1);
    const changed = await nextRuntimeEvent(events, "sessions.changed");
    expect(changed.type === "sessions.changed" && changed.sessionIds).toContain(created.id);

    const fetched = await client.sessions.get(sessionId);
    expect(fetched).toEqual(created);
    const updated = await client.sessions.update({
      sessionId,
      expectedRevision: created.revision,
      title: "HTTP contract e2e updated",
    });
    expect(updated.revision).toBe(created.revision + 1);

    const listed = await client.sessions.list();
    expect(listed.data.some((session) => session.id === created.id)).toBe(true);

    await client.sessions.delete(sessionId);
    await expect(client.sessions.get(sessionId)).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && errorType(error.data) === "session_not_found",
    );

    streamController.abort();
    await events.return?.();
  }, 30_000);

  it("applies provider credentials and model roles through the live resolvers", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const providerId = "openai-compatible";
    await expect(client.providers.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          id: providerId,
          requiresBaseUrl: true,
        }),
      ]),
    });
    await expect(client.models.list(providerId)).resolves.toMatchObject({
      data: [expect.objectContaining({ id: "e2e-model", provider: providerId })],
    });
    await expect(client.providers.test(providerId)).resolves.toEqual({ ok: true });

    const updated = await client.providers.update({
      provider: providerId,
      apiKey: { type: "set", value: "alpha-credential-omega" },
    });
    expect(updated).toMatchObject({
      id: providerId,
      apiKeyMasked: "al****ga",
      keySource: "stored",
    });
    expect(updated.apiKeyMasked).not.toContain("alpha-credential-omega");
    await expect(client.providers.test(providerId)).resolves.toEqual({ ok: true });

    await expect(
      client.models.setUtilityRole({ provider: providerId, model: "e2e-model" }),
    ).resolves.toEqual({ provider: providerId, model: "e2e-model" });
    await expect(client.models.getUtilityRole()).resolves.toEqual({
      provider: providerId,
      model: "e2e-model",
    });
    await expect(
      client.models.setEmbeddingRole({ provider: providerId, model: "e2e-model" }),
    ).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );
    await client.providers.update({
      provider: "openai",
      apiKey: { type: "set", value: "embedding-test-key" },
      baseUrl: { type: "set", value: providerBaseUrl },
    });
    await expect(
      client.models.setEmbeddingRole({ provider: "openai", model: "e2e-embedding" }),
    ).resolves.toEqual({ provider: "openai", model: "e2e-embedding" });
    await expect(client.models.getEmbeddingRole()).resolves.toEqual({
      provider: "openai",
      model: "e2e-embedding",
    });

    await expect(client.models.setUtilityRole({})).resolves.toEqual({});
    await expect(client.models.setEmbeddingRole({})).resolves.toEqual({});
    await expect(client.models.getUtilityRole()).resolves.toEqual({});
    await expect(client.models.getEmbeddingRole()).resolves.toEqual({});
  }, 30_000);

  it("streams a complete run and reconciles its durable state", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP run lifecycle",
    });
    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_RUN complete without tools." }],
    });
    const runId = asRunId(started.result.runId);
    const events = await collectRunEvents(started.events);

    expect(events.at(0)?.event.type).toBe("segment.started");
    expect(events.some((event) => event.event.type === "item.delta")).toBe(true);
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });
    await expect(client.runs.get(runId)).resolves.toMatchObject({
      id: runId,
      sessionId: session.id,
      status: "finished",
      outcome: { type: "completed" },
    });
    await expect(
      client.items.list({ scope: { type: "session", sessionId: session.id } }),
    ).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ type: "userMessage", runId: started.result.runId }),
        expect.objectContaining({ type: "agentMessage", runId: started.result.runId }),
      ]),
    });
    await expect(client.usage.session(asSessionId(session.id))).resolves.toMatchObject({
      inputTokens: expect.any(Number),
      outputTokens: expect.any(Number),
    });
  }, 30_000);

  it("reattaches to a live segment before releasing the in-flight model", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const gate = createProviderGate("E2E_SUBSCRIBE");
    providerGate = gate;
    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP run reattach",
      });
      const started = await client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_SUBSCRIBE hold the model request." }],
      });
      await within(gate.arrived.promise, "the reattach model request");

      const attached = await client.runs.subscribe({
        runId: asRunId(started.result.runId),
        segmentId: asSegmentId(started.result.segmentId),
      });
      expect(attached.result).toMatchObject({
        runId: started.result.runId,
        segmentId: started.result.segmentId,
        headEventId: expect.stringMatching(/^evt_/),
      });

      gate.release.resolve();
      const [openingEvents, attachedEvents] = await within(
        Promise.all([collectRunEvents(started.events), collectRunEvents(attached.events)]),
        "both run streams to finish",
      );
      expect(openingEvents.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        outcome: { type: "completed" },
      });
      expect(attachedEvents.some((event) => event.event.type === "item.delta")).toBe(true);
      expect(attachedEvents.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        outcome: { type: "completed" },
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
    }
  }, 30_000);

  it("applies steering only after the in-flight model boundary", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const gate = createProviderGate("E2E_STEER");
    providerGate = gate;
    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP run steering",
      });
      const started = await client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_STEER hold the first model boundary." }],
      });
      await within(gate.arrived.promise, "the steer model request");

      await client.runs.steer(
        asRunId(started.result.runId),
        asSegmentId(started.result.segmentId),
        [{ type: "text", text: "Include the queued steering instruction." }],
      );
      gate.release.resolve();

      const events = await within(
        collectRunEvents(started.events),
        "the steered run stream to finish",
      );
      const steeredMessage = events.find(
        (event) =>
          event.event.type === "item.completed" &&
          event.event.item.type === "userMessage" &&
          event.event.item.content?.some(
            (block) =>
              block.type === "text" && block.text === "Include the queued steering instruction.",
          ),
      );
      expect(steeredMessage).toBeDefined();
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        outcome: { type: "completed" },
      });
      await expect(
        client.items.list({ scope: { type: "run", runId: started.result.runId } }),
      ).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            type: "userMessage",
            content: expect.arrayContaining([
              { type: "text", text: "Include the queued steering instruction." },
            ]),
          }),
        ]),
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
    }
  }, 30_000);

  it("cancels an in-flight model request and durably closes its stream", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const gate = createProviderGate("E2E_CANCEL");
    providerGate = gate;
    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP run cancellation",
      });
      const started = await client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_CANCEL hold the model request." }],
      });
      const runId = asRunId(started.result.runId);
      await within(gate.arrived.promise, "the cancel model request");

      const canceled = await client.runs.cancel(runId, "HTTP cancellation E2E");
      expect(canceled).toMatchObject({
        type: "root",
        run: {
          id: runId,
          status: "finished",
          outcome: { type: "canceled", detail: "HTTP cancellation E2E" },
        },
      });
      await within(gate.closed.promise, "the canceled provider connection to close");

      const events = await within(
        collectRunEvents(started.events),
        "the canceled run stream to finish",
      );
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        outcome: { type: "canceled", detail: "HTTP cancellation E2E" },
      });
      await expect(client.runs.get(runId)).resolves.toMatchObject({
        id: runId,
        status: "finished",
        outcome: { type: "canceled", detail: "HTTP cancellation E2E" },
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
    }
  }, 30_000);

  it("forks a visible durable conversation at the selected run boundary", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const source = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP fork source",
    });
    const started = await client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_FORK preserve this visible turn." }],
    });
    const forkRunEvents = await collectRunEvents(started.events);
    expect(forkRunEvents.filter((event) => event.event.type === "item.completed")).toHaveLength(2);
    const sourceRuns = await client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const sourceItems = await client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(sourceRuns).toHaveLength(1);
    expect(sourceRuns[0]?.metrics.steps).toBe(1);
    expect(sourceItems).toHaveLength(2);

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["sessions.changed", "runs.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const fork = await client.sessions.fork({
      sessionId: asSessionId(source.id),
      fromRunId: asRunId(started.result.runId),
      title: "HTTP visible fork",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "sessions.changed")).resolves.toMatchObject({
      type: "sessions.changed",
      sessionIds: [fork.id],
    });
    await expect(nextRuntimeEvent(runtimeEvents, "runs.changed")).resolves.toMatchObject({
      type: "runs.changed",
      sessionIds: [fork.id],
      runIds: [expect.not.stringMatching(started.result.runId)],
    });
    const forkRuns = await client.runs
      .list({ sessionId: asSessionId(fork.id) })
      .autoPagingToArray();
    expect(forkRuns).toHaveLength(1);
    expect(forkRuns[0]).toMatchObject({ sessionId: fork.id, status: "finished" });
    expect(forkRuns[0]?.id).not.toBe(started.result.runId);
    const forkItems = await client.items
      .list({ scope: { type: "session", sessionId: asSessionId(fork.id) } })
      .autoPagingToArray();
    expect(forkItems).toHaveLength(2);
    expect(forkItems).toEqual([
      expect.objectContaining({
        type: "userMessage",
        content: [{ type: "text", text: "E2E_FORK preserve this visible turn." }],
      }),
      expect.objectContaining({
        type: "agentMessage",
        content: [{ type: "text", text: "HTTP runtime lifecycle complete." }],
      }),
    ]);
    expect(forkItems.every((item) => item.runId === forkRuns[0]?.id)).toBe(true);

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("round-trips exports, rollback state and aggregate invalidations", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const source = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP transfer lifecycle",
    });
    const first = await client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_PLAN preserve this plan in the archive." }],
    });
    await collectRunEvents(first.events);
    const second = await client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_TRANSFER remove and restore this turn." }],
    });
    await collectRunEvents(second.events);

    const jsonExport = await client.sessions.export(asSessionId(source.id), "json");
    if (!jsonExport.artifact) throw new Error("JSON export omitted its session artifact");
    expect(jsonExport).toMatchObject({ format: "json" });
    expect(jsonExport).not.toHaveProperty("markdown");
    expect(jsonExport.artifact).toMatchObject({
      session: { id: source.id, title: "HTTP transfer lifecycle" },
      runs: [{ id: first.result.runId }, { id: second.result.runId }],
      states: [
        {
          type: "plan",
          plan: [
            { description: "Inspect the runtime contract", status: "completed" },
            { description: "Verify frontend reconciliation", status: "in_progress" },
          ],
        },
      ],
    });
    expect(jsonExport.artifact.items.length).toBeGreaterThanOrEqual(5);

    const markdownExport = await client.sessions.export(asSessionId(source.id), "md");
    expect(markdownExport).toEqual({
      format: "md",
      markdown: expect.stringContaining("# HTTP transfer lifecycle"),
    });
    expect(markdownExport.markdown).toContain("E2E_PLAN preserve this plan in the archive.");
    expect(markdownExport.markdown).toContain("E2E_TRANSFER remove and restore this turn.");

    const aggregateTopics = [
      "sessions.changed",
      "runs.changed",
      "interrupts.changed",
      "goals.changed",
      "state.changed",
    ] as const;
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: [...aggregateTopics] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const rolledBack = await client.sessions.rollback({
      sessionId: asSessionId(source.id),
      toRunId: asRunId(first.result.runId),
      restoreType: "history",
    });
    expect(rolledBack.droppedRuns).toEqual([
      expect.objectContaining({
        run: expect.objectContaining({ id: second.result.runId }),
        userInput: [{ type: "text", text: "E2E_TRANSFER remove and restore this turn." }],
      }),
    ]);
    expect(
      (await collectSessionRuntimeEvents(runtimeEvents, aggregateTopics, source.id)).map(
        (event) => event.type,
      ),
    ).toEqual(aggregateTopics);
    const rolledRuns = await client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const rolledItems = await client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(rolledRuns.map((run) => run.id)).toEqual([first.result.runId]);
    expect(rolledItems.every((item) => item.runId === first.result.runId)).toBe(true);
    await expect(client.plan.get(asSessionId(source.id))).resolves.toMatchObject({
      plan: jsonExport.artifact.states?.[0]?.plan,
    });

    const imported = await client.sessions.import(jsonExport.artifact);
    expect(imported.session.id).toBe(source.id);
    expect(imported.session.revision).toBeGreaterThan(rolledBack.session.revision);
    expect(
      (await collectSessionRuntimeEvents(runtimeEvents, aggregateTopics, source.id)).map(
        (event) => event.type,
      ),
    ).toEqual(aggregateTopics);
    const restoredRuns = await client.runs
      .list({ sessionId: asSessionId(source.id) })
      .autoPagingToArray();
    const restoredItems = await client.items
      .list({ scope: { type: "session", sessionId: asSessionId(source.id) } })
      .autoPagingToArray();
    expect(restoredRuns.map((run) => run.id)).toEqual([second.result.runId, first.result.runId]);
    expect(restoredItems).toHaveLength(jsonExport.artifact.items.length);
    await expect(client.plan.get(asSessionId(source.id))).resolves.toMatchObject({
      plan: jsonExport.artifact.states?.[0]?.plan,
    });

    const continued = await client.runs.start({
      sessionId: asSessionId(source.id),
      input: [{ type: "text", text: "E2E_TRANSFER continue after import." }],
    });
    const continuedEvents = await collectRunEvents(continued.events);
    expect(continuedEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("publishes plan state on the stream and through the exact cold read", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP plan lifecycle",
    });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["state.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_PLAN publish the two-step plan." }],
    });
    const events = await collectRunEvents(started.events);
    expect(
      events.some(
        (event) =>
          event.event.type === "state.snapshot" &&
          event.event.state.type === "plan" &&
          event.event.state.plan.length === 2,
      ),
    ).toBe(true);

    const changed = await nextRuntimeEvent(runtimeEvents, "state.changed");
    expect(changed).toMatchObject({
      type: "state.changed",
      key: "plan",
      sessionIds: [session.id],
    });
    await expect(client.plan.get(asSessionId(session.id))).resolves.toMatchObject({
      type: "plan",
      revision: 1,
      sessionId: session.id,
      plan: [
        { description: "Inspect the runtime contract", status: "completed" },
        { description: "Verify frontend reconciliation", status: "in_progress" },
      ],
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("parks and resumes the same run through durable HITL identity", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP HITL lifecycle",
    });
    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_HITL ask before continuing." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(started.events);
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "interrupt" },
    });
    const waiting = await client.runs.get(runId);
    expect(waiting).toMatchObject({ status: "waiting" });
    expect(waiting).not.toHaveProperty("activeSegmentId");

    const pending = await client.interrupts.list({ rootRunId: runId });
    expect(pending.data).toHaveLength(1);
    expect(pending.data[0]?.interrupts).toHaveLength(1);
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("runtime did not persist the expected question interrupt");
    }
    expect(question.runId).toBe(started.result.runId);
    expect(question.payload.question.fields[0]?.prompt).toBe("Continue the HTTP lifecycle check?");

    const resumed = await client.runs.resume({
      runId,
      responses: [
        {
          itemId: question.itemId,
          response: { type: "answer", answers: [["Yes"]] },
        },
      ],
    });
    expect(resumed.result.runId).toBe(started.result.runId);
    expect(resumed.result.segmentId).not.toBe(started.result.segmentId);
    const resumeEvents = await collectRunEvents(resumed.events);
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });
    await expect(client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({ data: [] });
    await expect(client.runs.get(runId)).resolves.toMatchObject({
      status: "finished",
      outcome: { type: "completed" },
    });
  }, 30_000);

  it("opens and clears an approval interrupt around the exact tool call", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP approval lifecycle",
    });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["interrupts.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_APPROVAL run the command after approval." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(started.events);
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "interrupt" },
    });
    await expect(nextRuntimeEvent(runtimeEvents, "interrupts.changed")).resolves.toMatchObject({
      type: "interrupts.changed",
      runIds: [runId],
      sessionIds: [session.id],
    });

    const pending = await client.interrupts.list({ rootRunId: runId });
    const approval = pending.data[0]?.interrupts[0];
    if (!approval || approval.type !== "approval") {
      throw new Error("runtime did not persist the expected approval interrupt");
    }
    expect(approval.runId).toBe(runId);
    expect(approval.payload.rememberable).toBe(true);
    expect(approval.payload.tool).toMatchObject({
      name: "shell",
      arguments: {
        command: "printf approval-ok",
        description: "Print the approval marker",
      },
    });

    const resumed = await client.runs.resume({
      runId,
      responses: [
        {
          itemId: approval.itemId,
          response: {
            type: "approval",
            decision: "approve",
            remember: { scope: "session" },
          },
        },
      ],
    });
    const resumeEvents = await collectRunEvents(resumed.events);
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });
    await expect(nextRuntimeEvent(runtimeEvents, "interrupts.changed")).resolves.toMatchObject({
      type: "interrupts.changed",
      runIds: [runId],
      sessionIds: [session.id],
    });
    await expect(client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({ data: [] });

    const rules = await client.approval.listRules(asSessionId(session.id));
    expect(rules.rules).toEqual([
      expect.objectContaining({ scope: "session", tool: "shell", decision: "allow" }),
    ]);
    const rememberedRule = rules.rules[0];
    if (!rememberedRule) throw new Error("approval decision was not remembered");
    await client.approval.forgetRule(rememberedRule.id);
    await expect(client.approval.listRules(asSessionId(session.id))).resolves.toEqual({
      rules: [],
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("round-trips the runtime approval mode without changing run policy", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const original = await client.approval.getMode();
    const alternate = original.mode === "yolo" ? "balanced" : "yolo";
    await expect(client.approval.setMode(alternate)).resolves.toEqual({ mode: alternate });
    await expect(client.approval.getMode()).resolves.toEqual({ mode: alternate });
    await expect(client.approval.setMode(original.mode)).resolves.toEqual(original);
    await expect(client.approval.getMode()).resolves.toEqual(original);
  }, 30_000);

  it("drives a goal to its durable budget boundary", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP goal lifecycle",
    });
    const sessionId = asSessionId(session.id);
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["goals.changed", "runs.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    await expect(
      client.goals.start({
        sessionId,
        objective: "E2E_GOAL complete one autonomous run.",
        budget: { maxRuns: 1 },
      }),
    ).resolves.toMatchObject({
      sessionId: session.id,
      status: "active",
      budget: { maxRuns: 1 },
      used: { runs: 0 },
    });
    await nextRuntimeEvent(runtimeEvents, "goals.changed");

    let current = await client.goals.get(sessionId);
    for (let attempt = 0; attempt < 100 && current?.status === "active"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      current = await client.goals.get(sessionId);
    }
    expect(current).toMatchObject({
      sessionId: session.id,
      status: "blocked",
      reason: { code: "runBudgetReached" },
      used: { runs: 1 },
    });
    await nextRuntimeEvent(runtimeEvents, "goals.changed");

    const runs = await client.runs.list({ sessionId });
    expect(runs.data).toHaveLength(1);
    expect(runs.data[0]).toMatchObject({
      sessionId: session.id,
      status: "finished",
      outcome: { type: "completed" },
    });
    await expect(client.goals.resume(sessionId)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("stops an active goal, cancels its owned run and resumes from durable usage", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const gate = createProviderGate("E2E_GOAL_STOP");
    providerGate = gate;
    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP goal stop and resume",
      });
      const sessionId = asSessionId(session.id);
      await expect(
        client.goals.start({
          sessionId,
          objective: "E2E_GOAL_STOP exercise stop and resume.",
          budget: { maxRuns: 2 },
        }),
      ).resolves.toMatchObject({ status: "active", used: { runs: 0 } });
      await within(gate.arrived.promise, "the goal-owned model request");

      await expect(client.goals.stop(sessionId)).resolves.toMatchObject({
        sessionId: session.id,
        status: "paused",
        reason: { code: "stoppedByUser" },
        used: { runs: 1 },
      });
      await within(gate.closed.promise, "the stopped goal provider connection to close");
      gate.release.resolve();
      providerGate = undefined;
      await expect(client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            outcome: expect.objectContaining({ type: "canceled" }),
            status: "finished",
          }),
        ],
      });

      await expect(client.goals.resume(sessionId)).resolves.toMatchObject({
        sessionId: session.id,
        status: "active",
        used: { runs: 1 },
      });
      let current = await client.goals.get(sessionId);
      for (let attempt = 0; attempt < 100 && current?.status === "active"; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 50));
        current = await client.goals.get(sessionId);
      }
      expect(current).toMatchObject({
        status: "blocked",
        reason: { code: "runBudgetReached" },
        used: { runs: 2 },
      });
      const runs = await client.runs.list({ sessionId });
      expect(runs.data).toHaveLength(2);
      expect(runs.data.filter((run) => run.outcome?.type === "canceled")).toHaveLength(1);
      expect(runs.data.filter((run) => run.outcome?.type === "completed")).toHaveLength(1);
    } finally {
      gate.release.resolve();
      providerGate = undefined;
    }
  }, 30_000);

  it("reconciles the schedule lifecycle through schedules.changed", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["schedules.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const created = await client.schedules.create({
      cron: "0 0 1 1 *",
      instructions: "Run the annual HTTP E2E check.",
      title: "HTTP schedule lifecycle",
      workspace: { path: root },
    });
    await expect(nextRuntimeEvent(runtimeEvents, "schedules.changed")).resolves.toMatchObject({
      type: "schedules.changed",
      scheduleIds: [created.id],
    });
    await expect(client.schedules.list()).resolves.toMatchObject({
      data: [expect.objectContaining({ id: created.id, revision: created.revision })],
    });

    const fired = await client.schedules.runNow(created.id);
    expect(fired.sessionId).toEqual(expect.stringMatching(/^ses_/));
    expect(fired.runId).toEqual(expect.stringMatching(/^run_/));
    await expect(nextRuntimeEvent(runtimeEvents, "schedules.changed")).resolves.toMatchObject({
      type: "schedules.changed",
      scheduleIds: [created.id],
    });
    let firedRun = await client.runs.get(asRunId(fired.runId));
    for (let attempt = 0; attempt < 100 && firedRun.status !== "finished"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      firedRun = await client.runs.get(asRunId(fired.runId));
    }
    expect(firedRun).toMatchObject({
      id: fired.runId,
      sessionId: fired.sessionId,
      status: "finished",
      outcome: { type: "completed" },
    });
    const firedSchedule = (await client.schedules.list()).data[0];
    expect(firedSchedule).toMatchObject({
      id: created.id,
      lastRunAt: expect.any(String),
    });
    if (!firedSchedule) throw new Error("schedule disappeared after its manual run");
    expect(firedSchedule.revision).toBeGreaterThan(created.revision);

    const updated = await client.schedules.update({
      id: created.id,
      expectedRevision: firedSchedule.revision,
      enabled: false,
      title: "HTTP schedule lifecycle updated",
    });
    expect(updated.revision).toBe(firedSchedule.revision + 1);
    await expect(nextRuntimeEvent(runtimeEvents, "schedules.changed")).resolves.toMatchObject({
      type: "schedules.changed",
      scheduleIds: [created.id],
    });

    await client.schedules.delete(created.id);
    await expect(nextRuntimeEvent(runtimeEvents, "schedules.changed")).resolves.toMatchObject({
      type: "schedules.changed",
      scheduleIds: [created.id],
    });
    await expect(client.schedules.list()).resolves.toMatchObject({ data: [] });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("reconciles disabled MCP configuration through mcp.changed", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["mcp.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const server = "http-e2e-disabled";

    const created = await client.mcp.create({
      connection: { type: "stdio", command: "runtime-http-e2e-mcp" },
      description: "Disabled MCP E2E fixture",
      enabled: false,
      name: server,
    });
    expect(created).toMatchObject({ name: server, status: { type: "disabled" } });
    await expect(nextRuntimeEvent(runtimeEvents, "mcp.changed")).resolves.toMatchObject({
      type: "mcp.changed",
      serverIds: [server],
    });
    await expect(client.mcp.list()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: server, status: { type: "disabled" } })],
    });

    const updated = await client.mcp.update({
      server,
      description: "Updated disabled MCP E2E fixture",
    });
    expect(updated).toMatchObject({
      name: server,
      description: "Updated disabled MCP E2E fixture",
      status: { type: "disabled" },
    });
    await expect(nextRuntimeEvent(runtimeEvents, "mcp.changed")).resolves.toMatchObject({
      type: "mcp.changed",
      serverIds: [server],
    });

    await client.mcp.delete(server);
    await expect(nextRuntimeEvent(runtimeEvents, "mcp.changed")).resolves.toMatchObject({
      type: "mcp.changed",
      serverIds: [server],
    });
    await expect(client.mcp.list()).resolves.toMatchObject({ data: [] });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("reconciles skill archive and restore through skills.changed", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspace = client.workspace({ path: root });
    await expect(client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, scope: "user" })],
    });

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["skills.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    await client.skills.archive(managedSkillName);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "archived" })],
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({ data: [] });

    await client.skills.restore(managedSkillName);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, scope: "user" })],
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("reconciles an agent file write through files.changed", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-agent-write");
    await mkdir(workspaceRoot);
    await writeFile(join(workspaceRoot, "agent-write.txt"), "before\n");
    const session = await client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP agent file write",
    });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["files.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_FILE_WRITE update the fixture." }],
    });
    const events = await collectRunEvents(started.events);
    expect(events.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });

    const changed = await nextRuntimeEvent(runtimeEvents, "files.changed");
    expect(changed).toMatchObject({
      type: "files.changed",
      paths: ["agent-write.txt"],
      workspace: session.workspace.ref,
    });
    const file = await client
      .workspace(session.workspace.ref)
      .files.read({ path: "agent-write.txt" });
    if (file.content !== "after\n") {
      const toolEvents = events.filter(
        (event) => event.event.type === "item.completed" && event.event.item.type === "toolCall",
      );
      throw new Error(`agent write did not persist: ${JSON.stringify(toolEvents)}`);
    }
    expect(file.path).toBe("agent-write.txt");

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("keeps file-change signals scoped and reconciles them through cold reads", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-file-events");
    await mkdir(workspaceRoot);
    await execFileAsync("git", ["init", "--quiet"], { cwd: workspaceRoot });
    await execFileAsync("git", ["config", "user.name", "Runtime HTTP E2E"], {
      cwd: workspaceRoot,
    });
    await execFileAsync("git", ["config", "user.email", "runtime-http-e2e@example.invalid"], {
      cwd: workspaceRoot,
    });
    await writeFile(join(workspaceRoot, "tracked.txt"), "before\n");
    await execFileAsync("git", ["add", "tracked.txt"], { cwd: workspaceRoot });
    await execFileAsync("git", ["commit", "--quiet", "-m", "baseline"], {
      cwd: workspaceRoot,
    });

    const streamController = new AbortController();
    const watchId = "workspace-file-events";
    const subscription = await client.runtimeEvents.subscribe(
      {
        topics: ["files.changed"],
        watches: [{ watchId, workspace: { path: workspaceRoot } }],
      },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    await writeFile(join(workspaceRoot, "tracked.txt"), "after\n");
    await execFileAsync("git", ["add", "tracked.txt"], { cwd: workspaceRoot });

    const changed = await nextRuntimeEvent(events, "resync");
    expect(changed.type === "resync" && changed.topics).toContain("files.changed");
    expect(changed.type === "resync" && changed.watchIds).toContain(watchId);

    const workspace = client.workspace({ path: workspaceRoot });
    await expect(workspace.files.read({ path: "tracked.txt" })).resolves.toMatchObject({
      content: "after\n",
      path: "tracked.txt",
    });
    await expect(workspace.changes.list()).resolves.toMatchObject({
      data: [{ path: "tracked.txt", status: "modified" }],
    });
    await expect(workspace.diff.get()).resolves.toMatchObject({
      files: [{ path: "tracked.txt", status: "modified" }],
    });

    streamController.abort();
    await events.return?.();
  }, 30_000);

  it("preserves the home, project-root and workspace knowledge cascade", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const projectRoot = join(root, "workspace-knowledge-project");
    const workspaceRoot = join(projectRoot, "packages", "desktop");
    await mkdir(workspaceRoot, { recursive: true });
    await execFileAsync("git", ["init", "--quiet"], { cwd: projectRoot });
    const workspace = client.workspace({ path: workspaceRoot });

    await workspace.knowledge.update({ scope: "home", content: "home knowledge\n" });
    await workspace.knowledge.update({
      scope: "projectRoot",
      content: "project-root knowledge\n",
    });
    await workspace.knowledge.update({ scope: "cwd", content: "workspace knowledge\n" });

    await expect(workspace.knowledge.get("home")).resolves.toEqual({
      scope: "home",
      content: "home knowledge\n",
    });
    await expect(workspace.knowledge.get("projectRoot")).resolves.toEqual({
      scope: "projectRoot",
      content: "project-root knowledge\n",
    });
    await expect(workspace.knowledge.get("cwd")).resolves.toEqual({
      scope: "cwd",
      content: "workspace knowledge\n",
    });
    await expect(workspace.knowledge.list()).resolves.toMatchObject({
      data: [
        { scope: "home", content: "home knowledge\n", updatedAt: expect.any(String) },
        {
          scope: "projectRoot",
          content: "project-root knowledge\n",
          updatedAt: expect.any(String),
        },
        { scope: "cwd", content: "workspace knowledge\n", updatedAt: expect.any(String) },
      ],
    });
    await expect(readFile(join(projectRoot, "LYRA.md"), "utf8")).resolves.toBe(
      "project-root knowledge\n",
    );
    await expect(readFile(join(workspaceRoot, "LYRA.md"), "utf8")).resolves.toBe(
      "workspace knowledge\n",
    );

    await workspace.knowledge.update({ scope: "home", content: "" });
    await workspace.knowledge.update({ scope: "projectRoot", content: "" });
    await workspace.knowledge.update({ scope: "cwd", content: "" });
    await expect(workspace.knowledge.list()).resolves.toMatchObject({ data: [] });
  }, 30_000);

  it("round-trips project and user agent memory through durable cold reads", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-agent-memory");
    await mkdir(workspaceRoot);
    const workspace = client.workspace({ path: workspaceRoot });
    const project = await workspace.agentMemory.add("project memory marker");
    expect(project).toMatchObject({
      scope: "project",
      content: "project memory marker",
      origin: "user",
      status: "active",
      pinned: false,
    });
    const duplicate = await workspace.agentMemory.add("project memory marker");
    expect(duplicate.id).toBe(project.id);

    const user = await client.agentMemory.add({
      scope: "user",
      content: "user memory marker",
    });
    expect(user).toMatchObject({ scope: "user", origin: "user", status: "active" });
    await expect(workspace.agentMemory.list()).resolves.toMatchObject({
      items: [expect.objectContaining({ id: project.id, content: "project memory marker" })],
    });
    await expect(client.agentMemory.list({ scope: "user" })).resolves.toMatchObject({
      items: [expect.objectContaining({ id: user.id, content: "user memory marker" })],
    });

    const updated = await client.agentMemory.update({
      id: project.id,
      content: "project memory marker updated",
      pinned: true,
    });
    expect(updated).toMatchObject({
      id: project.id,
      content: "project memory marker updated",
      pinned: true,
    });
    await expect(workspace.agentMemory.list()).resolves.toMatchObject({
      items: [
        expect.objectContaining({
          id: project.id,
          content: "project memory marker updated",
          pinned: true,
        }),
      ],
    });
    await expect(client.agentMemory.review(project.id, "approve")).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );

    await client.agentMemory.delete(project.id);
    await client.agentMemory.delete(user.id);
    expect((await workspace.agentMemory.list()).items.some((item) => item.id === project.id)).toBe(
      false,
    );
    expect(
      (await client.agentMemory.list({ scope: "user" })).items.some((item) => item.id === user.id),
    ).toBe(false);
  }, 30_000);

  it("aggregates durable usage and accepts write-only feedback", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP usage and feedback",
    });
    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [{ type: "text", text: "E2E_USAGE produce metered output." }],
    });
    await collectRunEvents(started.events);
    await expect(client.runs.get(asRunId(started.result.runId))).resolves.toMatchObject({
      provider: "openai-compatible",
      model: "e2e-model",
      status: "finished",
    });
    const items = await client.items
      .list({ scope: { type: "run", runId: asRunId(started.result.runId) } })
      .autoPagingToArray();
    const agentItem = items.find((item) => item.type === "agentMessage");
    if (!agentItem) throw new Error("metered run omitted its agent item");

    await expect(
      client.feedback.create({
        sessionId: session.id,
        runId: started.result.runId,
        itemId: agentItem.id,
        rating: "positive",
        text: "HTTP feedback marker",
      }),
    ).resolves.toBeUndefined();
    await expect(client.feedback.create({})).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );

    const summary = await client.usage.summary({ sinceDays: 30 });
    expect(summary).toMatchObject({
      total: {
        inputTokens: expect.any(Number),
        outputTokens: expect.any(Number),
      },
      sessions: expect.any(Number),
      runs: expect.any(Number),
      byProvider: expect.arrayContaining([
        expect.objectContaining({ key: "openai-compatible", runs: expect.any(Number) }),
      ]),
      byModel: expect.arrayContaining([
        expect.objectContaining({ key: "openai-compatible/e2e-model", runs: expect.any(Number) }),
      ]),
    });
    expect(summary.total.inputTokens).toBeGreaterThan(0);
    expect(summary.total.outputTokens).toBeGreaterThan(0);
    expect(summary.sessions).toBeGreaterThan(0);
    expect(summary.runs).toBeGreaterThan(0);
    await expect(client.usage.summary({ sinceDays: -1 })).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );
  }, 30_000);

  it("invokes the direct diagnostic catalog inside its admitted workspace", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-direct-tools");
    await mkdir(workspaceRoot);
    await writeFile(join(workspaceRoot, "diagnostic.txt"), "direct-tool-marker\n");
    const session = await client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP direct diagnostic tools",
    });

    await expect(client.workspaces.resolve(session.workspace.ref)).resolves.toMatchObject({
      ref: session.workspace.ref,
    });
    await expect(client.workspaces.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          workspace: expect.objectContaining({ ref: session.workspace.ref }),
        }),
      ]),
    });
    await expect(client.tools.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ name: "read", safetyClass: "safe" }),
        expect.objectContaining({ name: "glob", safetyClass: "safe" }),
        expect.objectContaining({ name: "grep", safetyClass: "safe" }),
      ]),
    });

    const readResult = await client.tools.invoke({
      name: "read",
      arguments: { path: "diagnostic.txt" },
      workspace: session.workspace.ref,
    });
    expect(JSON.stringify(readResult)).toContain("direct-tool-marker");
    const grepResult = await client.tools.invoke({
      name: "grep",
      arguments: { pattern: "direct-tool-marker", path: "." },
      workspace: session.workspace.ref,
    });
    expect(JSON.stringify(grepResult)).toContain("diagnostic.txt");

    await expect(
      client.tools.invoke({
        name: "read",
        arguments: { path: "../outside.txt" },
        workspace: session.workspace.ref,
      }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && errorType(error.data) === "path_outside_root",
    );
  }, 30_000);
});
