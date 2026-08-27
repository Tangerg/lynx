// @vitest-environment node

import { execFile } from "node:child_process";
import {
  chmod,
  lstat,
  mkdir,
  mkdtemp,
  readFile,
  realpath,
  rm,
  stat,
  symlink,
  writeFile,
} from "node:fs/promises";
import {
  createServer as createHttpServer,
  type IncomingMessage,
  request as requestHttp,
  type ServerResponse,
} from "node:http";
import { createServer as createNetServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { promisify } from "node:util";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { createScopeAppClient, type ScopeAppClient } from "./sdk";
import { RpcError } from "./errors";
import { asRunId, asSegmentId, asSessionId } from "./ids";
import { errorType } from "./types";
import { createSidecarClient } from "./sidecar";
import { createHttpTransport } from "./transports/http";
import { isWireStreamingMethodName, type WireMethodName } from "@scopeapp/runtime-contract/methods";
import {
  PROTOCOL_VERSION,
  type RequestMeta,
  type RunEvent,
  type RuntimeEvent,
} from "@scopeapp/runtime-contract/wire";

const execFileAsync = promisify(execFile);
const runtimeDirectory = resolve(process.cwd(), "../../runtime");
const managedSkillName = "runtime-http-e2e";
const liveMCPToolName = "http-e2e-stdio_ping";
const killedMCPToolName = "http-e2e-kill_ping";

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
  input?: string | string[];
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
  minimumToolResults: number;
  release: Deferred;
}

function deferred(): Deferred {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function createProviderGate(marker: string, minimumToolResults = 0): ProviderGate {
  return {
    arrived: deferred(),
    claimed: false,
    closed: deferred(),
    marker,
    minimumToolResults,
    release: deferred(),
  };
}

function testDeadline(detail: string): { promise: Promise<never>; release: () => void } {
  let settled = false;
  let release!: () => void;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const promise = new Promise<never>((resolve, reject) => {
    release = () => {
      if (settled) return;
      settled = true;
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
      resolve(undefined as never);
    };
    timer = setTimeout(() => {
      timer = undefined;
      settled = true;
      reject(new Error(`timed out waiting for ${detail}`));
    }, 5_000);
  });
  return { promise, release };
}

async function within<T>(promise: Promise<T>, detail: string): Promise<T> {
  const deadline = testDeadline(detail);
  try {
    return await Promise.race([promise, deadline.promise]);
  } finally {
    deadline.release();
  }
}

async function requestJson(request: IncomingMessage): Promise<FakeChatRequest> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as FakeChatRequest;
}

type UnaryCutpoint = "beforeCommit" | "afterCommit";

function faultUnaryResponse(
  method: string,
  cutpoint: UnaryCutpoint,
): { attempts: () => number; fetch: typeof fetch } {
  let attempts = 0;
  let injected = false;
  const faultFetch: typeof fetch = async (input, init) => {
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as { method?: string }) : {};
    if (body.method !== method) return globalThis.fetch(input, init);
    attempts += 1;
    if (injected) return globalThis.fetch(input, init);
    injected = true;
    if (cutpoint === "beforeCommit") {
      throw new Error("injected disconnect before Runtime admission");
    }

    const committed = await globalThis.fetch(input, init);
    await committed.arrayBuffer();
    const lostBody = new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.error(new Error("injected response body loss after Runtime commit"));
      },
    });
    return new Response(lostBody, {
      headers: committed.headers,
      status: committed.status,
      statusText: committed.statusText,
    });
  };
  return { attempts: () => attempts, fetch: faultFetch };
}

const isolatedFetch: typeof fetch = async (input, init) => {
  const payload =
    typeof init?.body === "string" ? (JSON.parse(init.body) as { method?: string }) : {};
  if (payload.method && isWireStreamingMethodName(payload.method as WireMethodName)) {
    return globalThis.fetch(input, init);
  }
  const headers = new Headers(init?.headers);
  headers.set("Connection", "close");
  const url =
    typeof input === "string" || input instanceof URL ? new URL(input) : new URL(input.url);
  return new Promise<Response>((resolve, reject) => {
    const request = requestHttp(
      url,
      {
        method: init?.method,
        headers: Object.fromEntries(headers.entries()),
        signal: init?.signal ?? undefined,
      },
      (response) => {
        const chunks: Buffer[] = [];
        response.on("data", (chunk: Buffer) => chunks.push(chunk));
        response.once("error", reject);
        response.once("end", () => {
          const responseHeaders = new Headers();
          for (const [name, value] of Object.entries(response.headers)) {
            if (Array.isArray(value)) {
              for (const item of value) responseHeaders.append(name, item);
            } else if (value !== undefined) {
              responseHeaders.set(name, value);
            }
          }
          resolve(
            new Response(Buffer.concat(chunks), {
              status: response.statusCode ?? 500,
              statusText: response.statusMessage,
              headers: responseHeaders,
            }),
          );
        });
      },
    );
    request.once("error", reject);
    if (typeof init?.body === "string" || init?.body instanceof Uint8Array) {
      request.end(init.body);
    } else {
      request.end();
    }
  });
};

function faultStreamOpening(method: string): { attempts: () => number; fetch: typeof fetch } {
  let attempts = 0;
  let injected = false;
  const faultFetch: typeof fetch = async (input, init) => {
    const body =
      typeof init?.body === "string" ? (JSON.parse(init.body) as { method?: string }) : {};
    if (body.method !== method) return globalThis.fetch(input, init);
    attempts += 1;
    const committed = await globalThis.fetch(input, init);
    if (injected) return committed;
    injected = true;
    if (!(committed.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
      throw new Error(`expected ${method} to return an event stream`);
    }
    await committed.body?.cancel("injected opening response loss after Runtime commit");
    const lostBody = new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.error(new Error("injected opening response loss after Runtime commit"));
      },
    });
    return new Response(lostBody, {
      headers: committed.headers,
      status: committed.status,
      statusText: committed.statusText,
    });
  };
  return { attempts: () => attempts, fetch: faultFetch };
}

function scriptedReply(body: FakeChatRequest): {
  text?: string;
  tool?: FakeToolCall;
  tools?: FakeToolCall[];
} {
  const transcript = JSON.stringify(body.messages ?? []);
  const availableTools = new Set(
    (body.tools ?? []).flatMap((tool) => (tool.function?.name ? [tool.function.name] : [])),
  );
  const toolResultCount = (body.messages ?? []).filter((message) => message.role === "tool").length;
  const hasToolResult = toolResultCount > 0;

  if (
    transcript.includes("E2E_GOAL_SETTLEMENT") &&
    availableTools.has("report_goal_outcome") &&
    !hasToolResult
  ) {
    return {
      tool: {
        name: "report_goal_outcome",
        arguments: JSON.stringify({ outcome: "completed" }),
      },
    };
  }

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
  if (
    transcript.includes("E2E_PARALLEL_EDITED_TOOL") &&
    availableTools.has("shell") &&
    !hasToolResult
  ) {
    return {
      tools: [
        {
          name: "shell",
          arguments: JSON.stringify({
            command: "printf approval-original",
            description: "Print the original approval marker",
          }),
        },
        {
          name: "shell",
          arguments: JSON.stringify({
            command: "printf approval-sibling",
            description: "Print the sibling approval marker",
          }),
        },
      ],
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
  if (
    (transcript.includes("E2E_SKILL_PROPOSAL_PROJECT") ||
      transcript.includes("E2E_SKILL_PROPOSAL_USER")) &&
    availableTools.has("search_tools") &&
    !availableTools.has("propose_skill") &&
    toolResultCount === 0
  ) {
    return {
      tool: {
        name: "search_tools",
        arguments: JSON.stringify({ query: "select:propose_skill" }),
      },
    };
  }
  if (
    transcript.includes("E2E_SKILL_PROPOSAL_PROJECT") &&
    availableTools.has("propose_skill") &&
    toolResultCount === 1
  ) {
    return {
      tool: {
        name: "propose_skill",
        arguments: JSON.stringify({
          name: "e2e-project-proposal",
          description: "Preserve the project-side HTTP E2E workflow.",
          instructions: "Run the project-side HTTP E2E workflow and report its result.",
          scope: "project",
        }),
      },
    };
  }
  if (
    transcript.includes("E2E_SKILL_PROPOSAL_USER") &&
    availableTools.has("propose_skill") &&
    toolResultCount === 1
  ) {
    return {
      tool: {
        name: "propose_skill",
        arguments: JSON.stringify({
          name: "e2e-user-proposal",
          description: "Preserve the user-side HTTP E2E workflow.",
          instructions: "Run the user-side HTTP E2E workflow and report its result.",
          scope: "user",
        }),
      },
    };
  }
  if (
    transcript.includes("E2E_MCP_TOOL") &&
    availableTools.has("search_tools") &&
    !availableTools.has(liveMCPToolName) &&
    toolResultCount === 0
  ) {
    return {
      tool: {
        name: "search_tools",
        arguments: JSON.stringify({ query: `select:${liveMCPToolName}` }),
      },
    };
  }
  if (
    transcript.includes("E2E_MCP_TOOL") &&
    availableTools.has(liveMCPToolName) &&
    toolResultCount === 1
  ) {
    return {
      tool: {
        name: liveMCPToolName,
        arguments: "{}",
      },
    };
  }
  if (transcript.includes("E2E_MCP_TOOL") && toolResultCount >= 2) {
    return { text: transcript.includes("pong") ? "MCP pong observed." : "MCP pong missing." };
  }
  if (
    transcript.includes("E2E_FORCE_KILL_TOOL") &&
    availableTools.has("search_tools") &&
    !availableTools.has(killedMCPToolName) &&
    toolResultCount === 0
  ) {
    return {
      tool: {
        name: "search_tools",
        arguments: JSON.stringify({ query: `select:${killedMCPToolName}` }),
      },
    };
  }
  if (transcript.includes("E2E_FORCE_KILL_TOOL") && toolResultCount >= 1) {
    return { tool: { name: killedMCPToolName, arguments: "{}" } };
  }
  return { text: "HTTP runtime lifecycle complete." };
}

function fakeEmbedding(text: string): number[] {
  const normalized = text.toLowerCase();
  if (normalized.includes("semantic target marker")) return [1, 0, 0];
  if (normalized.includes("unrelated helper")) return [0, 1, 0];
  return [0, 0, 1];
}

function writeChatCompletion(
  response: ServerResponse,
  body: FakeChatRequest,
  sequence: number,
): void {
  const id = `chatcmpl-e2e-${sequence}`;
  const model = body.model ?? "e2e-model";
  const reply = scriptedReply(body);
  const tools = reply.tools ?? (reply.tool ? [reply.tool] : []);
  const finishReason = tools.length > 0 ? "tool_calls" : "stop";
  const callID = (index: number) =>
    index === 0 ? `call-e2e-${sequence}` : `call-e2e-${sequence}-${index}`;
  const message =
    tools.length > 0
      ? {
          role: "assistant",
          content: "",
          tool_calls: tools.map((tool, index) => ({
            id: callID(index),
            type: "function",
            function: { name: tool.name, arguments: tool.arguments },
          })),
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
  const chunks =
    tools.length > 0
      ? [
          { choices: [{ index: 0, delta: { role: "assistant" }, finish_reason: null }] },
          {
            choices: [
              {
                index: 0,
                delta: {
                  tool_calls: tools.map((tool, index) => ({
                    index,
                    id: callID(index),
                    type: "function",
                    function: { name: tool.name, arguments: tool.arguments },
                  })),
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
      await response.arrayBuffer();
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
  const deadline = testDeadline(type);
  try {
    while (true) {
      const result = await Promise.race([iterator.next(), deadline.promise]);
      if (result.done) throw new Error(`runtime event stream ended before ${type}`);
      if (result.value.type === type) return result.value;
    }
  } finally {
    deadline.release();
  }
}

async function collectSessionRuntimeEvents(
  iterator: AsyncIterator<RuntimeEvent>,
  types: readonly RuntimeEvent["type"][],
  sessionId: string,
): Promise<RuntimeEvent[]> {
  const pending = new Set<RuntimeEvent["type"]>(types);
  const collected: RuntimeEvent[] = [];
  const deadline = testDeadline(`aggregate reconciliation: ${types.join(", ")}`);
  try {
    while (pending.size > 0) {
      const result = await Promise.race([iterator.next(), deadline.promise]);
      if (result.done)
        throw new Error("runtime event stream ended before aggregate reconciliation");
      const event = result.value;
      if (
        pending.has(event.type) &&
        "sessionIds" in event &&
        event.sessionIds?.includes(sessionId)
      ) {
        pending.delete(event.type);
        collected.push(event);
      }
    }
    return collected;
  } finally {
    deadline.release();
  }
}

async function collectRunEvents(events: AsyncIterable<RunEvent>): Promise<RunEvent[]> {
  const collected: RunEvent[] = [];
  for await (const event of events) collected.push(event);
  return collected;
}

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK", () => {
  let environmentRoot = "";
  let root = "";
  let runtimeHome = "";
  let runtimeData = "";
  let baseUrl = "";
  let mcpFixturePath = "";
  let providerBaseUrl = "";
  let runtime: ReturnType<typeof import("node:child_process").spawn> | undefined;
  let provider: ReturnType<typeof createHttpServer> | undefined;
  let providerGate: ProviderGate | undefined;
  let client: ScopeAppClient | undefined;
  let processOutput = "";
  let runtimeExecutable = "";
  let runtimePort = 0;

  const runtimeEnvironment = () => ({
    ...process.env,
    HOME: runtimeHome,
    SCOPEAPP_HOME: runtimeData,
    SCOPEAPP_PROVIDER: "openai-compatible",
    SCOPEAPP_MODEL: "e2e-model",
    SCOPEAPP_APIKEY: "e2e-placeholder-key",
    SCOPEAPP_BASEURL: providerBaseUrl,
    OPENAI_COMPATIBLE_API_KEY: "e2e-placeholder-key",
    OPENAI_API_KEY: "",
    SCOPEAPP_SERVER_LISTEN: `127.0.0.1:${runtimePort}`,
    SCOPEAPP_SERVER_NOLOCALTOKEN: "true",
    SCOPEAPP_MCP_SERVERS: "",
    SCOPEAPP_A2A_AGENTS: "",
    OTEL_SDK_DISABLED: "true",
  });

  const startRuntimeProcess = async () => {
    const { spawn } = await import("node:child_process");
    processOutput = "";
    runtime = spawn(runtimeExecutable, [], {
      cwd: root,
      env: runtimeEnvironment(),
      stdio: ["ignore", "pipe", "pipe"],
    });
    runtime.stdout?.on("data", (chunk: Buffer) => {
      processOutput += chunk.toString();
    });
    runtime.stderr?.on("data", (chunk: Buffer) => {
      processOutput += chunk.toString();
    });
    await waitUntilReady(baseUrl, () => processOutput);
  };

  const stopRuntimeProcess = async () => {
    const child = runtime;
    if (!child || child.exitCode !== null) return;
    const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
    child.kill("SIGTERM");
    try {
      await within(exited, "Runtime graceful shutdown");
    } catch {
      if (child.exitCode === null) child.kill("SIGKILL");
      await within(exited, "Runtime forced shutdown");
    }
  };

  const killRuntimeProcess = async () => {
    const child = runtime;
    if (!child || child.exitCode !== null) return;
    const exited = new Promise<void>((resolve) => child.once("exit", () => resolve()));
    if (!child.kill("SIGKILL")) throw new Error("failed to send SIGKILL to Runtime");
    await within(exited, "Runtime SIGKILL exit");
  };

  const createRuntimeClient = () =>
    createScopeAppClient(createHttpTransport({ baseUrl, fetch: isolatedFetch }), {
      requestMeta: () => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "runtime-http-e2e", version: "1" },
        clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
      }),
    });

  beforeAll(async () => {
    environmentRoot = await mkdtemp(join(tmpdir(), "scopeapp-runtime-e2e-"));
    root = join(environmentRoot, "workspace");
    runtimeHome = join(environmentRoot, "home");
    runtimeData = join(environmentRoot, "runtime-data");
    await Promise.all([mkdir(root, { recursive: true }), mkdir(runtimeHome, { recursive: true })]);
    mcpFixturePath = join(environmentRoot, "mcp-fixture.mjs");
    await writeFile(
      mcpFixturePath,
      `import { writeFileSync } from "node:fs";
import { createInterface } from "node:readline";

const lines = createInterface({ input: process.stdin, crlfDelay: Infinity });
const holdToolCallMarker = process.argv.find((value) => value.startsWith("--hold-tool-call="))?.slice("--hold-tool-call=".length);
const reply = (id, result) => {
  process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id, result }) + "\\n");
};

for await (const line of lines) {
  const message = JSON.parse(line);
  if (message.id === undefined) continue;
  if (message.method === "initialize") {
    reply(message.id, {
      protocolVersion: message.params?.protocolVersion ?? "2025-11-25",
      capabilities: { tools: {} },
      serverInfo: { name: "runtime-http-e2e-mcp", version: "1.0.0" },
    });
    continue;
  }
  if (message.method === "tools/list") {
    reply(message.id, {
      tools: [{
        name: "ping",
        description: "Returns pong for Runtime HTTP E2E.",
        inputSchema: { type: "object", properties: {}, additionalProperties: false },
      }],
    });
    continue;
  }
  if (message.method === "tools/call") {
    if (holdToolCallMarker) {
      writeFileSync(holdToolCallMarker, "arrived\\n");
      continue;
    }
    reply(message.id, { content: [{ type: "text", text: "pong" }] });
    continue;
  }
  reply(message.id, { });
}
`,
    );
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
        if (request.method === "POST" && request.url?.endsWith("/embeddings")) {
          const body = await requestJson(request);
          const inputs = Array.isArray(body.input) ? body.input : [body.input ?? ""];
          response.writeHead(200, { "Content-Type": "application/json" });
          response.end(
            JSON.stringify({
              object: "list",
              model: body.model ?? "e2e-embedding",
              data: inputs.map((input, index) => ({
                object: "embedding",
                index,
                embedding: fakeEmbedding(input),
              })),
              usage: { prompt_tokens: inputs.length, total_tokens: inputs.length },
            }),
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
          JSON.stringify(body.messages ?? []).includes(gate.marker) &&
          (body.messages ?? []).filter((message) => message.role === "tool").length >=
            gate.minimumToolResults
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

    runtimeExecutable = join(environmentRoot, "scopeapp-e2e");
    await execFileAsync("go", ["build", "-o", runtimeExecutable, "./cmd/scopeapp"], {
      cwd: runtimeDirectory,
    });

    runtimePort = await unusedLoopbackPort();
    baseUrl = `http://127.0.0.1:${runtimePort}`;
    await startRuntimeProcess();
    client = createRuntimeClient();
  }, 60_000);

  afterAll(async () => {
    await client?.close();
    await stopRuntimeProcess();
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
    const liveness = await sidecar.liveness();
    const readiness = await sidecar.readiness();
    const info = await sidecar.info();
    expect(liveness).toMatchObject({ status: "ok" });
    expect(readiness).toMatchObject({ status: "ok" });
    expect(info).toMatchObject({
      protocolVersion: PROTOCOL_VERSION,
      transport: "http",
    });

    const discovery = await client.runtime.discover();
    expect([
      info.server.instanceId,
      liveness.instanceId,
      readiness.instanceId,
      discovery.serverInfo.instanceId,
    ]).toEqual(Array(4).fill(discovery.serverInfo.instanceId));
    expect(discovery.protocolVersion).toBe(PROTOCOL_VERSION);
    expect(discovery.capabilities.limits.idempotency.namespace).toMatch(/^idp_[0-9a-f]{32}$/);
    const repeatDiscovery = await client.runtime.discover();
    expect(repeatDiscovery).toMatchObject({
      capabilities: {
        limits: {
          idempotency: { namespace: discovery.capabilities.limits.idempotency.namespace },
        },
      },
    });
    expect(repeatDiscovery.serverInfo.instanceId).toBe(discovery.serverInfo.instanceId);
    expect(discovery.capabilities.streamingMethods).toContain("runtime.subscribe");

    const staleStoreTitle = "must not enter replacement Runtime";
    const staleStoreResponse = await isolatedFetch(`${baseUrl}/v2/rpc`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "Idempotency-Key": "stale-store-session-create",
        "Idempotency-Namespace": "idp_00000000000000000000000000000000",
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: "stale-store-fence",
        method: "sessions.create",
        params: { workspace: { path: root }, title: staleStoreTitle },
      }),
    });
    expect(staleStoreResponse.status).toBe(200);
    await expect(staleStoreResponse.json()).resolves.toMatchObject({
      error: { data: { type: "idempotency_store_mismatch" } },
    });
    const sessionsAfterStoreMismatch = await client.sessions.list();
    expect(
      sessionsAfterStoreMismatch.data.some((session) => session.title === staleStoreTitle),
    ).toBe(false);

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

  it("converges unary mutations across pre-admission and post-commit disconnects", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const createdIds: string[] = [];

    try {
      for (const cutpoint of ["beforeCommit", "afterCommit"] as const) {
        const fault = faultUnaryResponse("sessions.create", cutpoint);
        const faultClient = createScopeAppClient(
          createHttpTransport({ baseUrl, fetch: fault.fetch }),
          {
            requestMeta,
          },
        );
        try {
          const created = await faultClient.sessions.create({
            workspace: { path: root },
            title: `HTTP ${cutpoint} replay`,
          });
          createdIds.push(created.id);
          expect(fault.attempts()).toBe(2);
          await expect(client.sessions.get(asSessionId(created.id))).resolves.toEqual(created);
          await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
            type: "sessions.changed",
            sessionIds: [created.id],
          });
        } finally {
          await faultClient.close();
        }
      }

      const sessions = await client.sessions.list().autoPagingToArray();
      for (const id of createdIds) {
        expect(sessions.filter((session) => session.id === id)).toHaveLength(1);
      }

      const deletedId = createdIds.pop();
      if (!deletedId) throw new Error("post-commit Session was not created");
      const fault = faultUnaryResponse("sessions.delete", "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        await faultClient.sessions.delete(asSessionId(deletedId));
        expect(fault.attempts()).toBe(2);
      } finally {
        await faultClient.close();
      }
      await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
        type: "sessions.changed",
        sessionIds: [deletedId],
      });
      await expect(client.sessions.get(asSessionId(deletedId))).rejects.toSatisfy(
        (error: unknown) =>
          error instanceof RpcError && errorType(error.data) === "session_not_found",
      );
    } finally {
      for (const id of createdIds) await client.sessions.delete(asSessionId(id));
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reattaches one committed Run after its streaming acknowledgement is lost", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP streaming replay cutpoint",
    });
    const gate = createProviderGate("E2E_RUN_REPLAY_CUTPOINT");
    providerGate = gate;
    const fault = faultStreamOpening("runs.start");
    const faultClient = createScopeAppClient(createHttpTransport({ baseUrl, fetch: fault.fetch }), {
      requestMeta: (): RequestMeta => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "runtime-http-stream-cutpoint-e2e", version: "1" },
        clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
      }),
    });

    try {
      const opening = faultClient.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_RUN_REPLAY_CUTPOINT finish exactly once." }],
      });
      await within(gate.arrived.promise, "the replay-cutpoint model request");
      const started = await opening;
      expect(fault.attempts()).toBe(2);

      gate.release.resolve();
      const events = await collectRunEvents(started.events);
      expect(events.at(-1)?.event).toMatchObject({
        type: "segment.finished",
        outcome: { type: "completed" },
      });
      const runs = await client.runs
        .list({ sessionId: asSessionId(session.id) })
        .autoPagingToArray();
      expect(runs).toHaveLength(1);
      expect(runs[0]).toMatchObject({
        id: started.result.runId,
        sessionId: session.id,
        status: "finished",
        outcome: { type: "completed" },
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
      await faultClient.close();
      await client.sessions.delete(asSessionId(session.id));
    }
  }, 30_000);

  it("reconciles Schedule response and acknowledgement cutpoints through cold reads", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-schedule-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: ScopeAppClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["schedules.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    let scheduleId: string | undefined;

    try {
      const created = await callAfterCommitLoss("schedules.create", (faultClient) =>
        faultClient.schedules.create({
          cron: "0 0 1 1 *",
          instructions: "Exercise post-commit Schedule replay.",
          title: "HTTP Schedule replay cutpoint",
          workspace: { path: root },
        }),
      );
      scheduleId = created.id;
      await expect(nextRuntimeEvent(events, "schedules.changed")).resolves.toMatchObject({
        type: "schedules.changed",
        scheduleIds: [created.id],
      });
      await expect(client.schedules.list()).resolves.toMatchObject({
        data: [expect.objectContaining({ id: created.id, revision: created.revision })],
      });

      const fired = await callAfterCommitLoss("schedules.runNow", (faultClient) =>
        faultClient.schedules.runNow(created.id),
      );
      await expect(nextRuntimeEvent(events, "schedules.changed")).resolves.toMatchObject({
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
      });

      const current = (await client.schedules.list()).data.find(
        (schedule) => schedule.id === created.id,
      );
      if (!current) throw new Error("Schedule disappeared before replayed update");
      const updated = await callAfterCommitLoss("schedules.update", (faultClient) =>
        faultClient.schedules.update({
          id: created.id,
          enabled: false,
          expectedRevision: current.revision,
          title: "HTTP Schedule replay cutpoint updated",
          workspaceMode: "default",
        }),
      );
      await expect(nextRuntimeEvent(events, "schedules.changed")).resolves.toMatchObject({
        type: "schedules.changed",
        scheduleIds: [created.id],
      });
      await expect(client.schedules.list()).resolves.toMatchObject({
        data: [
          expect.objectContaining({ id: created.id, revision: updated.revision, enabled: false }),
        ],
      });

      await callAfterCommitLoss("schedules.delete", (faultClient) =>
        faultClient.schedules.delete(created.id),
      );
      scheduleId = undefined;
      await expect(nextRuntimeEvent(events, "schedules.changed")).resolves.toMatchObject({
        type: "schedules.changed",
        scheduleIds: [created.id],
      });
      await expect(client.schedules.list()).resolves.toMatchObject({ data: [] });
    } finally {
      if (scheduleId) await client.schedules.delete(scheduleId).catch(() => undefined);
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("keeps one Goal drive across lost start, stop and resume responses", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-goal-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: ScopeAppClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP Goal replay cutpoints",
    });
    const sessionId = asSessionId(session.id);
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["goals.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const firstGate = createProviderGate("E2E_GOAL_REPLAY_CUTPOINT");
    let resumedGate: ProviderGate | undefined;
    providerGate = firstGate;

    try {
      await expect(
        callAfterCommitLoss("goals.start", (faultClient) =>
          faultClient.goals.start({
            sessionId,
            objective: "E2E_GOAL_REPLAY_CUTPOINT preserve one drive per command.",
            budget: { maxRuns: 3 },
          }),
        ),
      ).resolves.toMatchObject({ status: "active", used: { runs: 0 } });
      await within(firstGate.arrived.promise, "the replayed Goal's first model request");
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(client.goals.get(sessionId)).resolves.toMatchObject({
        status: "active",
        used: { runs: 0 },
      });
      await expect(client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [expect.objectContaining({ status: "running" })],
      });

      await expect(
        callAfterCommitLoss("goals.stop", (faultClient) => faultClient.goals.stop(sessionId)),
      ).resolves.toMatchObject({
        status: "paused",
        reason: { code: "stoppedByUser" },
        used: { runs: 1 },
      });
      await within(
        firstGate.closed.promise,
        "the replay-stopped Goal provider connection to close",
      );
      firstGate.release.resolve();
      providerGate = undefined;
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(client.goals.get(sessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "stoppedByUser" },
        used: { runs: 1 },
      });
      await expect(client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            status: "finished",
            outcome: expect.objectContaining({ type: "canceled" }),
          }),
        ],
      });

      resumedGate = createProviderGate("E2E_GOAL_REPLAY_CUTPOINT");
      providerGate = resumedGate;
      await expect(
        callAfterCommitLoss("goals.resume", (faultClient) => faultClient.goals.resume(sessionId)),
      ).resolves.toMatchObject({ status: "active", used: { runs: 1 } });
      await within(resumedGate.arrived.promise, "the replayed Goal's resumed model request");
      await expect(nextRuntimeEvent(events, "goals.changed")).resolves.toMatchObject({
        type: "goals.changed",
        sessionIds: [session.id],
      });
      await expect(client.goals.get(sessionId)).resolves.toMatchObject({
        status: "active",
        used: { runs: 1 },
      });
      const resumedRuns = await client.runs.list({ sessionId });
      expect(resumedRuns.data).toHaveLength(2);
      expect(resumedRuns.data.filter((run) => run.status === "running")).toHaveLength(1);
      expect(resumedRuns.data.filter((run) => run.outcome?.type === "canceled")).toHaveLength(1);

      await expect(client.goals.stop(sessionId)).resolves.toMatchObject({
        status: "paused",
        used: { runs: 2 },
      });
      await within(resumedGate.closed.promise, "the cleanup Goal provider connection to close");
    } finally {
      firstGate.release.resolve();
      resumedGate?.release.resolve();
      providerGate = undefined;
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("consumes one durable HITL answer when the resume stream acknowledgement is lost", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP HITL resume replay cutpoint",
    });
    const sessionId = asSessionId(session.id);
    const started = await client.runs.start({
      sessionId,
      input: [{ type: "text", text: "E2E_HITL ask before replaying the answer." }],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(started.events);
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "interrupt" },
    });
    const pending = await client.interrupts.list({ rootRunId: runId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("Runtime did not persist the replay-cutpoint question");
    }

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["interrupts.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const fault = faultStreamOpening("runs.resume");
    const faultClient = createScopeAppClient(createHttpTransport({ baseUrl, fetch: fault.fetch }), {
      requestMeta: (): RequestMeta => ({
        protocolVersion: PROTOCOL_VERSION,
        clientInfo: { name: "runtime-http-hitl-resume-cutpoint-e2e", version: "1" },
        clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
      }),
    });

    try {
      const resumed = await faultClient.runs.resume({
        runId,
        responses: [
          {
            itemId: question.itemId,
            response: { type: "answer", answers: [["Yes"]] },
          },
        ],
      });
      expect(fault.attempts()).toBe(2);
      expect(resumed.result.runId).toBe(started.result.runId);
      expect(resumed.result.segmentId).not.toBe(started.result.segmentId);
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
      await expect(client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({
        data: [],
      });
      const items = await client.items.list({ scope: { type: "run", runId } }).autoPagingToArray();
      expect(
        items.filter(
          (item) =>
            item.type === "question" &&
            item.status === "completed" &&
            JSON.stringify(item.question?.answers) === JSON.stringify([["Yes"]]),
        ),
      ).toHaveLength(1);
      await expect(client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            id: runId,
            status: "finished",
            outcome: { type: "completed" },
          }),
        ],
      });
    } finally {
      await faultClient.close();
      streamController.abort();
      await runtimeEvents.return?.();
    }
  }, 30_000);

  it("reconciles provider and model-role commits after their responses are lost", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-model-config-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: ScopeAppClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const originalUtility = await client.models.getUtilityRole();
    const originalEmbedding = await client.models.getEmbeddingRole();
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["models.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    try {
      const provider = await callAfterCommitLoss("providers.update", (faultClient) =>
        faultClient.providers.update({
          provider: "openai",
          apiKey: { type: "set", value: "cutpoint-provider-secret" },
          baseUrl: { type: "set", value: providerBaseUrl },
        }),
      );
      expect(provider).toMatchObject({ id: "openai", keySource: "stored" });
      expect(provider.apiKeyMasked).not.toContain("cutpoint-provider-secret");
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(client.providers.list()).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            id: "openai",
            apiKeyMasked: provider.apiKeyMasked,
            keySource: "stored",
          }),
        ]),
      });

      await expect(
        callAfterCommitLoss("models.setUtilityRole", (faultClient) =>
          faultClient.models.setUtilityRole({ provider: "openai", model: "e2e-model" }),
        ),
      ).resolves.toEqual({ provider: "openai", model: "e2e-model" });
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(client.models.getUtilityRole()).resolves.toEqual({
        provider: "openai",
        model: "e2e-model",
      });

      await expect(
        callAfterCommitLoss("models.setEmbeddingRole", (faultClient) =>
          faultClient.models.setEmbeddingRole({ provider: "openai", model: "e2e-embedding" }),
        ),
      ).resolves.toEqual({ provider: "openai", model: "e2e-embedding" });
      await expect(nextRuntimeEvent(events, "models.changed")).resolves.toMatchObject({
        type: "models.changed",
      });
      await expect(client.models.getEmbeddingRole()).resolves.toEqual({
        provider: "openai",
        model: "e2e-embedding",
      });
    } finally {
      await client.models.setUtilityRole(originalUtility);
      await client.models.setEmbeddingRole(originalEmbedding);
      await client.providers.update({
        provider: "openai",
        apiKey: { type: "clear" },
        baseUrl: { type: "clear" },
      });
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reconciles Knowledge and Agent Memory mutations after their responses are lost", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-workspace-state-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: ScopeAppClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const workspaceRoot = join(root, "workspace-state-replay-cutpoints");
    await mkdir(workspaceRoot);
    const workspace = client.workspace({ path: workspaceRoot });
    const initialKnowledge = await workspace.knowledge.get("cwd");
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["knowledge.changed", "agentMemory.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    let knowledgeRevision = initialKnowledge.revision;
    let memoryId: string | undefined;

    try {
      const saved = await callAfterCommitLoss("knowledge.update", (faultClient) =>
        faultClient.workspace({ path: workspaceRoot }).knowledge.update({
          scope: "cwd",
          content: "knowledge replay cutpoint\n",
          expectedRevision: initialKnowledge.revision,
        }),
      );
      knowledgeRevision = saved.revision;
      await expect(nextRuntimeEvent(events, "knowledge.changed")).resolves.toMatchObject({
        type: "knowledge.changed",
      });
      await expect(workspace.knowledge.get("cwd")).resolves.toMatchObject({
        scope: "cwd",
        content: "knowledge replay cutpoint\n",
        revision: saved.revision,
      });

      const added = await callAfterCommitLoss("agentMemory.add", (faultClient) =>
        faultClient.agentMemory.add({
          scope: "project",
          workspace: { path: workspaceRoot },
          content: "agent-memory replay cutpoint",
        }),
      );
      memoryId = added.id;
      await expect(nextRuntimeEvent(events, "agentMemory.changed")).resolves.toMatchObject({
        type: "agentMemory.changed",
      });
      await expect(workspace.agentMemory.list()).resolves.toMatchObject({
        items: [expect.objectContaining({ id: added.id, content: "agent-memory replay cutpoint" })],
      });

      const updated = await callAfterCommitLoss("agentMemory.update", (faultClient) =>
        faultClient.agentMemory.update({
          id: added.id,
          content: "agent-memory replay cutpoint updated",
          pinned: true,
        }),
      );
      await expect(nextRuntimeEvent(events, "agentMemory.changed")).resolves.toMatchObject({
        type: "agentMemory.changed",
      });
      await expect(workspace.agentMemory.list()).resolves.toMatchObject({
        items: [
          expect.objectContaining({
            id: added.id,
            content: "agent-memory replay cutpoint updated",
            pinned: true,
            updatedAt: updated.updatedAt,
          }),
        ],
      });

      await callAfterCommitLoss("agentMemory.delete", (faultClient) =>
        faultClient.agentMemory.delete(added.id),
      );
      memoryId = undefined;
      await expect(nextRuntimeEvent(events, "agentMemory.changed")).resolves.toMatchObject({
        type: "agentMemory.changed",
      });
      expect((await workspace.agentMemory.list()).items).toEqual([]);
    } finally {
      if (memoryId) await client.agentMemory.delete(memoryId).catch(() => undefined);
      await workspace.knowledge
        .update({ scope: "cwd", content: "", expectedRevision: knowledgeRevision })
        .catch(() => undefined);
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("reconciles MCP and managed-Skill moves after their responses are lost", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const requestMeta = (): RequestMeta => ({
      protocolVersion: PROTOCOL_VERSION,
      clientInfo: { name: "runtime-http-resource-cutpoint-e2e", version: "1" },
      clientCapabilities: { features: {}, interruptTypes: ["approval", "question"] },
    });
    const callAfterCommitLoss = async <T>(
      method: string,
      call: (faultClient: ScopeAppClient) => Promise<T>,
    ): Promise<T> => {
      const fault = faultUnaryResponse(method, "afterCommit");
      const faultClient = createScopeAppClient(
        createHttpTransport({ baseUrl, fetch: fault.fetch }),
        {
          requestMeta,
        },
      );
      try {
        const value = await call(faultClient);
        expect(fault.attempts()).toBe(2);
        return value;
      } finally {
        await faultClient.close();
      }
    };
    const server = "http-e2e-replay-cutpoint";
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["mcp.changed", "skills.changed"] },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();

    try {
      await expect(
        callAfterCommitLoss("mcp.servers.create", (faultClient) =>
          faultClient.mcp.create({
            connection: { type: "stdio", command: "runtime-http-e2e-replay-cutpoint" },
            description: "MCP replay cutpoint",
            enabled: false,
            name: server,
          }),
        ),
      ).resolves.toMatchObject({ name: server, status: { type: "disabled" } });
      await expect(nextRuntimeEvent(events, "mcp.changed")).resolves.toMatchObject({
        type: "mcp.changed",
        serverIds: [server],
      });
      expect((await client.mcp.list()).data.filter((entry) => entry.name === server)).toHaveLength(
        1,
      );

      await expect(
        callAfterCommitLoss("mcp.servers.update", (faultClient) =>
          faultClient.mcp.update({
            server,
            description: "MCP replay cutpoint updated",
          }),
        ),
      ).resolves.toMatchObject({
        name: server,
        description: "MCP replay cutpoint updated",
        status: { type: "disabled" },
      });
      await expect(nextRuntimeEvent(events, "mcp.changed")).resolves.toMatchObject({
        type: "mcp.changed",
        serverIds: [server],
      });

      await callAfterCommitLoss("mcp.servers.delete", (faultClient) =>
        faultClient.mcp.delete(server),
      );
      await expect(nextRuntimeEvent(events, "mcp.changed")).resolves.toMatchObject({
        type: "mcp.changed",
        serverIds: [server],
      });
      expect((await client.mcp.list()).data.some((entry) => entry.name === server)).toBe(false);

      await callAfterCommitLoss("skills.library.archive", (faultClient) =>
        faultClient.skills.archive(managedSkillName),
      );
      await expect(nextRuntimeEvent(events, "skills.changed")).resolves.toMatchObject({
        type: "skills.changed",
      });
      await expect(client.skills.listLibrary()).resolves.toMatchObject({
        data: [expect.objectContaining({ name: managedSkillName, lifecycle: "archived" })],
      });
      await expect(client.workspace({ path: root }).skills.listDiscovered()).resolves.toMatchObject(
        {
          data: [],
        },
      );

      await callAfterCommitLoss("skills.library.restore", (faultClient) =>
        faultClient.skills.restore(managedSkillName),
      );
      await expect(nextRuntimeEvent(events, "skills.changed")).resolves.toMatchObject({
        type: "skills.changed",
      });
      await expect(client.skills.listLibrary()).resolves.toMatchObject({
        data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
      });
    } finally {
      await client.mcp.delete(server).catch(() => undefined);
      const managed = (await client.skills.listLibrary()).data.find(
        (skill) => skill.name === managedSkillName,
      );
      if (managed?.lifecycle === "archived") {
        await client.skills.restore(managedSkillName).catch(() => undefined);
      }
      streamController.abort();
      await events.return?.();
    }
  }, 30_000);

  it("isolates concurrent runtime subscriptions on one HTTP client", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const sessionsController = new AbortController();
    const schedulesController = new AbortController();
    const sessionsSubscription = await client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      sessionsController.signal,
    );
    const schedulesSubscription = await client.runtimeEvents.subscribe(
      { topics: ["schedules.changed"] },
      schedulesController.signal,
    );
    const sessionEvents = sessionsSubscription.events[Symbol.asyncIterator]();
    const scheduleEvents = schedulesSubscription.events[Symbol.asyncIterator]();
    let sessionId: string | undefined;
    let scheduleId: string | undefined;

    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP isolated runtime subscriptions",
      });
      sessionId = session.id;
      await expect(
        within(sessionEvents.next(), "the session subscription's first event"),
      ).resolves.toMatchObject({
        done: false,
        value: { type: "sessions.changed", sessionIds: [session.id] },
      });

      const schedule = await client.schedules.create({
        cron: "0 0 1 1 *",
        instructions: "Verify concurrent runtime subscription isolation.",
        title: "HTTP isolated runtime subscriptions",
        workspace: { path: root },
      });
      scheduleId = schedule.id;
      await expect(
        within(scheduleEvents.next(), "the schedule subscription's first event"),
      ).resolves.toMatchObject({
        done: false,
        value: { type: "schedules.changed", scheduleIds: [schedule.id] },
      });

      await client.sessions.update({
        sessionId: asSessionId(session.id),
        expectedRevision: session.revision,
        title: "HTTP isolated runtime subscriptions updated",
      });
      await expect(
        within(sessionEvents.next(), "the session subscription's second event"),
      ).resolves.toMatchObject({
        done: false,
        value: { type: "sessions.changed", sessionIds: [session.id] },
      });
    } finally {
      if (scheduleId) await client.schedules.delete(scheduleId);
      if (sessionId) await client.sessions.delete(asSessionId(sessionId));
      sessionsController.abort();
      schedulesController.abort();
      await sessionEvents.return?.();
      await scheduleEvents.return?.();
    }
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

    // A role is durable configuration intent. Removing a credential makes the
    // role temporarily unavailable, but must not silently erase the user's
    // model choice; product clients join this read with providers.list.
    await client.providers.update({
      provider: "openai",
      apiKey: { type: "clear" },
    });
    await expect(client.models.getEmbeddingRole()).resolves.toEqual({
      provider: "openai",
      model: "e2e-embedding",
    });
    await expect(client.providers.list()).resolves.toMatchObject({
      data: expect.arrayContaining([expect.objectContaining({ id: "openai", apiKeyMasked: "" })]),
    });
    await client.providers.update({
      provider: "openai",
      apiKey: { type: "set", value: "embedding-test-key" },
      baseUrl: { type: "set", value: providerBaseUrl },
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
      plan: [
        { description: "Inspect the runtime contract", status: "completed" },
        { description: "Verify frontend reconciliation", status: "in_progress" },
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
      "plan.changed",
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
      steps: jsonExport.artifact.plan,
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
      steps: jsonExport.artifact.plan,
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

  it("publishes the Plan on the stream and through the exact cold read", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP plan lifecycle",
    });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["plan.changed"] },
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
        (event) => event.event.type === "plan.updated" && event.event.plan.steps.length === 2,
      ),
    ).toBe(true);

    const changed = await nextRuntimeEvent(runtimeEvents, "plan.changed");
    expect(changed).toMatchObject({
      type: "plan.changed",
      sessionIds: [session.id],
    });
    await expect(client.plan.get(asSessionId(session.id))).resolves.toMatchObject({
      revision: 1,
      sessionId: session.id,
      steps: [
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

  it("keeps one Tool Item when edited approval resumes beside a same-name sibling after SIGKILL", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP edited parallel approval identity",
    });
    const started = await client.runs.start({
      sessionId: asSessionId(session.id),
      input: [
        {
          type: "text",
          text: "E2E_PARALLEL_EDITED_TOOL run both commands after the first edited approval.",
        },
      ],
    });
    const runId = asRunId(started.result.runId);
    const startEvents = await collectRunEvents(started.events);
    expect(startEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "interrupt" },
    });

    const pending = await client.interrupts.list({ rootRunId: runId });
    expect(pending.data).toHaveLength(1);
    expect(pending.data[0]?.interrupts).toHaveLength(1);
    const approval = pending.data[0]?.interrupts[0];
    if (!approval || approval.type !== "approval") {
      throw new Error("runtime did not persist the expected first parallel approval");
    }
    expect(approval.payload.tool).toMatchObject({
      name: "shell",
      arguments: { command: "printf approval-original" },
    });

    await client.close();
    client = undefined;
    await killRuntimeProcess();
    await startRuntimeProcess();
    client = createRuntimeClient();

    const resumed = await client.runs.resume({
      runId,
      responses: [
        {
          itemId: approval.itemId,
          response: {
            type: "approval",
            decision: "approve",
            editedArgs: {
              command: "printf approval-edited",
              description: "Print the edited approval marker",
            },
            remember: { scope: "session" },
          },
        },
      ],
    });
    const firstResumeEvents = await collectRunEvents(resumed.events);
    expect(firstResumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "interrupt" },
    });

    const firstLifecycle = [...startEvents, ...firstResumeEvents];
    const starts = firstLifecycle.filter(
      (event) => event.event.type === "item.started" && event.event.item.id === approval.itemId,
    );
    const completions = firstLifecycle.filter(
      (event) => event.event.type === "item.completed" && event.event.item.id === approval.itemId,
    );
    expect(starts).toHaveLength(1);
    expect(completions).toHaveLength(1);
    const completedEvent = completions[0]?.event;
    if (completedEvent?.type !== "item.completed" || completedEvent.item.type !== "toolCall") {
      throw new Error("edited approval did not complete the original Tool Item");
    }
    expect(completedEvent.item).toMatchObject({
      approvalDecision: "approve",
      id: approval.itemId,
      status: "completed",
      tool: {
        arguments: { command: "printf approval-edited" },
      },
    });
    expect(JSON.stringify(completedEvent.item.tool?.result)).toContain("approval-edited");

    const secondPending = await client.interrupts.list({ rootRunId: runId });
    expect(secondPending.data).toHaveLength(1);
    expect(secondPending.data[0]?.interrupts).toHaveLength(1);
    const siblingApproval = secondPending.data[0]?.interrupts[0];
    if (!siblingApproval || siblingApproval.type !== "approval") {
      throw new Error("runtime did not persist the same-name sibling approval");
    }
    expect(siblingApproval.itemId).not.toBe(approval.itemId);
    expect(siblingApproval.payload.tool).toMatchObject({
      name: "shell",
      arguments: { command: "printf approval-sibling" },
    });

    const completed = await client.runs.resume({
      runId,
      responses: [
        {
          itemId: siblingApproval.itemId,
          response: { type: "approval", decision: "approve" },
        },
      ],
    });
    const finalEvents = await collectRunEvents(completed.events);
    expect(finalEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });
    await expect(client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({ data: [] });

    const rules = await client.approval.listRules(asSessionId(session.id));
    await Promise.all(rules.rules.map((rule) => client?.approval.forgetRule(rule.id)));
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

  it("publishes a readable completing goal while final Run settlement is pending", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const gate = createProviderGate("E2E_GOAL_SETTLEMENT", 1);
    providerGate = gate;
    const streamController = new AbortController();
    let runtimeEvents: AsyncIterator<RuntimeEvent> | undefined;
    try {
      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP goal settlement lifecycle",
      });
      const sessionId = asSessionId(session.id);
      const subscription = await client.runtimeEvents.subscribe(
        { topics: ["goals.changed"] },
        streamController.signal,
      );
      runtimeEvents = subscription.events[Symbol.asyncIterator]();

      await expect(
        client.goals.start({
          sessionId,
          objective: "E2E_GOAL_SETTLEMENT expose the terminal settlement window.",
        }),
      ).resolves.toMatchObject({ status: "active" });
      await nextRuntimeEvent(runtimeEvents, "goals.changed");

      await within(gate.arrived.promise, "the post-outcome model request");
      await nextRuntimeEvent(runtimeEvents, "goals.changed");
      const completing = await client.goals.get(sessionId);
      expect(completing).toMatchObject({
        sessionId: session.id,
        status: "completing",
      });
      expect(completing).not.toHaveProperty("reason");

      gate.release.resolve();
      providerGate = undefined;
      let current = await client.goals.get(sessionId);
      for (let attempt = 0; attempt < 100 && current !== null; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 50));
        current = await client.goals.get(sessionId);
      }
      expect(current).toBeNull();
      await expect(client.runs.list({ sessionId })).resolves.toMatchObject({
        data: [
          expect.objectContaining({
            status: "finished",
            outcome: { type: "completed" },
          }),
        ],
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
      streamController.abort();
      await runtimeEvents?.return?.();
    }
  }, 30_000);

  it("parks and resumes a goal run through the negotiated HITL capability", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const session = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP goal HITL lifecycle",
    });
    const sessionId = asSessionId(session.id);
    await expect(
      client.goals.start({
        sessionId,
        objective: "E2E_HITL attempt to ask for input from this headless goal run.",
        budget: { maxRuns: 1 },
      }),
    ).resolves.toMatchObject({
      sessionId: session.id,
      status: "active",
      used: { runs: 0 },
    });

    let current = await client.goals.get(sessionId);
    for (let attempt = 0; attempt < 100 && current?.status === "active"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      current = await client.goals.get(sessionId);
    }
    expect(current).toMatchObject({
      sessionId: session.id,
      status: "paused",
      reason: { code: "awaitingInput" },
      used: { runs: 0 },
    });

    const runs = await client.runs.list({ sessionId });
    expect(runs.data).toEqual([
      expect.objectContaining({
        sessionId: session.id,
        status: "waiting",
      }),
    ]);
    const ownedRun = runs.data[0];
    if (!ownedRun) throw new Error("goal-owned waiting run disappeared");
    const runId = asRunId(ownedRun.id);
    const pending = await client.interrupts.list({ rootRunId: runId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("goal-owned run did not persist its question interrupt");
    }

    // Resume the Goal drive while its owned Run still occupies the Session. The
    // drive waits for that exact Run and must not admit a replacement.
    await expect(client.goals.resume(sessionId)).resolves.toMatchObject({
      sessionId: session.id,
      status: "active",
    });

    const resumed = await client.runs.resume({
      runId,
      responses: [
        {
          itemId: question.itemId,
          response: { type: "answer", answers: [["Yes"]] },
        },
      ],
    });
    const resumeEvents = await collectRunEvents(resumed.events);
    expect(resumeEvents.at(-1)?.event).toMatchObject({
      type: "segment.finished",
      outcome: { type: "completed" },
    });
    await expect(client.interrupts.list({ rootRunId: runId })).resolves.toMatchObject({ data: [] });
    await expect(client.items.list({ scope: { type: "run", runId } })).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          type: "question",
          status: "completed",
          question: expect.objectContaining({ answers: [["Yes"]] }),
        }),
      ]),
    });

    current = await client.goals.get(sessionId);
    for (let attempt = 0; attempt < 100 && current?.status === "active"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      current = await client.goals.get(sessionId);
    }
    expect(current).toMatchObject({
      status: "blocked",
      reason: { code: "runBudgetReached" },
      used: { runs: 1 },
    });
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
      workspaceMode: "default",
    });
    expect(updated.revision).toBe(firedSchedule.revision + 1);
    expect(updated.workspace).toBeUndefined();
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

  it("preserves filters and ordering across every high-cardinality cursor", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const timelineSession = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP pagination timeline",
    });
    const siblingSessions = await Promise.all([
      client.sessions.create({ workspace: { path: root }, title: "HTTP pagination sibling A" }),
      client.sessions.create({ workspace: { path: root }, title: "HTTP pagination sibling B" }),
    ]);
    const firstRun = await client.runs.start({
      sessionId: asSessionId(timelineSession.id),
      input: [{ type: "text", text: "E2E_PAGE first filtered run." }],
    });
    await collectRunEvents(firstRun.events);
    const secondRun = await client.runs.start({
      sessionId: asSessionId(timelineSession.id),
      input: [{ type: "text", text: "E2E_PAGE second filtered run." }],
    });
    await collectRunEvents(secondRun.events);

    const sessionIds = new Set([
      timelineSession.id,
      ...siblingSessions.map((session) => session.id),
    ]);
    const pagedSessions = await client.sessions.list({ limit: 1 }).autoPagingToArray();
    expect(pagedSessions.filter((session) => sessionIds.has(session.id))).toHaveLength(3);

    const pagedRuns = await client.runs
      .list({ sessionId: asSessionId(timelineSession.id), limit: 1 })
      .autoPagingToArray();
    expect(pagedRuns.map((run) => run.id)).toEqual([secondRun.result.runId, firstRun.result.runId]);
    expect(pagedRuns.every((run) => run.sessionId === timelineSession.id)).toBe(true);

    const pagedItems = await client.items
      .list({
        scope: { type: "session", sessionId: asSessionId(timelineSession.id) },
        limit: 1,
      })
      .autoPagingToArray();
    expect(pagedItems).toHaveLength(4);
    expect(pagedItems.map((item) => item.runId)).toEqual([
      firstRun.result.runId,
      firstRun.result.runId,
      secondRun.result.runId,
      secondRun.result.runId,
    ]);

    const waitingRuns: string[] = [];
    const schedules: string[] = [];
    try {
      for (const suffix of ["A", "B"]) {
        const waitingSession = await client.sessions.create({
          workspace: { path: root },
          title: `HTTP paged interrupt ${suffix}`,
        });
        const waiting = await client.runs.start({
          sessionId: asSessionId(waitingSession.id),
          input: [{ type: "text", text: `E2E_HITL pagination interrupt ${suffix}.` }],
        });
        const events = await collectRunEvents(waiting.events);
        expect(events.at(-1)?.event).toMatchObject({
          type: "segment.finished",
          outcome: { type: "interrupt" },
        });
        waitingRuns.push(waiting.result.runId);
      }
      const pagedInterrupts = await client.interrupts.list({ limit: 1 }).autoPagingToArray();
      expect(
        pagedInterrupts
          .filter((set) => waitingRuns.includes(set.rootRunId))
          .map((set) => set.rootRunId)
          .sort(),
      ).toEqual([...waitingRuns].sort());

      for (const suffix of ["A", "B"]) {
        const schedule = await client.schedules.create({
          cron: "0 0 1 1 *",
          instructions: `Run paged schedule ${suffix}.`,
          title: `HTTP paged schedule ${suffix}`,
          workspace: { path: root },
        });
        schedules.push(schedule.id);
      }
      const pagedSchedules = await client.schedules.list({ limit: 1 }).autoPagingToArray();
      expect(
        pagedSchedules
          .filter((schedule) => schedules.includes(schedule.id))
          .map((schedule) => schedule.id)
          .sort(),
      ).toEqual([...schedules].sort());
    } finally {
      for (const runId of waitingRuns) {
        await client.runs.cancel(asRunId(runId), "HTTP pagination cleanup");
      }
      for (const scheduleId of schedules) {
        await client.schedules.delete(scheduleId);
      }
    }
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

  it("probes, connects, lists and reconnects a real stdio MCP server", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const server = "http-e2e-stdio";
    const candidate = {
      autoApproveTools: ["ping"],
      connection: { type: "stdio" as const, command: process.execPath, args: [mcpFixturePath] },
      description: "Live MCP E2E fixture",
      enabled: true,
      name: server,
    };

    await expect(client.mcp.test(candidate)).resolves.toEqual({ ok: true });

    const connectController = new AbortController();
    const connectSubscription = await client.runtimeEvents.subscribe(
      { topics: ["mcp.changed"] },
      connectController.signal,
    );
    const connectEvents = connectSubscription.events[Symbol.asyncIterator]();
    await expect(client.mcp.create(candidate)).resolves.toMatchObject({ name: server });
    for (let phase = 0; phase < 2; phase++) {
      await expect(nextRuntimeEvent(connectEvents, "mcp.changed")).resolves.toMatchObject({
        type: "mcp.changed",
        serverIds: [server],
      });
    }

    let connected = (await client.mcp.list()).data.find((entry) => entry.name === server);
    for (let attempt = 0; attempt < 100 && connected?.status.type !== "connected"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 25));
      connected = (await client.mcp.list()).data.find((entry) => entry.name === server);
    }
    expect(connected).toMatchObject({
      name: server,
      status: { type: "connected", toolCount: 1 },
    });
    const allTools = await client.mcp.listTools();
    expect(allTools).toMatchObject({
      data: [
        {
          server,
          name: "ping",
          description: "Returns pong for Runtime HTTP E2E.",
          inputSchema: { type: "object", properties: {}, additionalProperties: false },
        },
      ],
    });
    await expect(client.mcp.listTools(server)).resolves.toMatchObject({
      data: [expect.objectContaining({ server, name: "ping" })],
    });

    const runSession = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP live MCP tool",
    });
    const mcpRun = await client.runs.start({
      sessionId: asSessionId(runSession.id),
      input: [{ type: "text", text: "E2E_MCP_TOOL invoke the live ping capability." }],
    });
    const mcpRunEvents = await collectRunEvents(mcpRun.events);
    expect(mcpRunEvents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: expect.objectContaining({
            type: "item.completed",
            item: expect.objectContaining({
              type: "toolCall",
              tool: expect.objectContaining({ name: liveMCPToolName }),
            }),
          }),
        }),
      ]),
    );
    await expect(
      client.items.list({ scope: { type: "run", runId: mcpRun.result.runId } }),
    ).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          type: "agentMessage",
          content: [{ type: "text", text: "MCP pong observed." }],
        }),
      ]),
    });

    await expect(client.mcp.authorizationAttempts.create(server)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );
    await expect(client.mcp.authorizationAttempts.get("mcpauth_missing")).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError &&
        errorType(error.data) === "mcp_authorization_attempt_not_found",
    );

    connectController.abort();
    await connectEvents.return?.();

    const reconnectController = new AbortController();
    const reconnectSubscription = await client.runtimeEvents.subscribe(
      { topics: ["mcp.changed"] },
      reconnectController.signal,
    );
    const reconnectEvents = reconnectSubscription.events[Symbol.asyncIterator]();
    await client.mcp.reconnect(server);
    for (let phase = 0; phase < 2; phase++) {
      await expect(nextRuntimeEvent(reconnectEvents, "mcp.changed")).resolves.toMatchObject({
        type: "mcp.changed",
        serverIds: [server],
      });
    }
    await expect(client.mcp.listTools(server)).resolves.toMatchObject({
      data: [expect.objectContaining({ server, name: "ping" })],
    });

    await client.mcp.delete(server);
    await expect(nextRuntimeEvent(reconnectEvents, "mcp.changed")).resolves.toMatchObject({
      type: "mcp.changed",
      serverIds: [server],
    });
    await expect(client.mcp.listTools(server)).resolves.toMatchObject({ data: [] });

    reconnectController.abort();
    await reconnectEvents.return?.();
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
      {
        topics: ["skills.changed", "files.changed"],
        watches: [{ watchId: "skills-authored", workspace: { path: root } }],
      },
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

    const projectSkillName = "external-project-skill";
    const projectSkillDirectory = join(root, ".scopeapp", "skills", projectSkillName);
    await mkdir(projectSkillDirectory, { recursive: true });
    await writeFile(
      join(projectSkillDirectory, "SKILL.md"),
      `---\nname: ${projectSkillName}\ndescription: Observe an externally authored project Skill.\n---\n\nUse the project workflow.\n`,
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ name: projectSkillName, scope: "project" }),
      ]),
    });

    const userSkillName = "external-user-skill";
    const userSkillDirectory = join(runtimeData, "skills", userSkillName);
    await mkdir(userSkillDirectory, { recursive: true });
    await writeFile(
      join(userSkillDirectory, "SKILL.md"),
      `---\nname: ${userSkillName}\ndescription: Observe an externally authored user Skill.\n---\n\nUse the user workflow.\n`,
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(client.skills.listLibrary()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ name: userSkillName, lifecycle: "active" }),
      ]),
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("reviews project and user skill proposals produced by a real run tool", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-skill-proposals");
    await mkdir(workspaceRoot);
    const workspace = client.workspace({ path: workspaceRoot });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["skills.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const projectSession = await client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP project Skill proposal",
    });
    const projectRun = await client.runs.start({
      sessionId: asSessionId(projectSession.id),
      input: [{ type: "text", text: "E2E_SKILL_PROPOSAL_PROJECT save this workflow." }],
    });
    const projectEvents = await collectRunEvents(projectRun.events);
    expect(projectEvents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: expect.objectContaining({
            type: "item.completed",
            item: expect.objectContaining({
              type: "toolCall",
              tool: expect.objectContaining({ name: "propose_skill" }),
            }),
          }),
        }),
      ]),
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });

    const projectProposals = await workspace.skills.listProposals();
    expect(projectProposals.data).toEqual([
      expect.objectContaining({
        name: "e2e-project-proposal",
        scope: "project",
        origin: "requested",
        sourceSession: projectSession.id,
        description: "Preserve the project-side HTTP E2E workflow.",
        instructions: "Run the project-side HTTP E2E workflow and report its result.",
        revision: expect.stringMatching(/^[a-f0-9]+$/),
      }),
    ]);
    const projectProposal = projectProposals.data[0];
    if (!projectProposal) throw new Error("project proposal disappeared before review");
    const projectRef = {
      name: projectProposal.name,
      revision: projectProposal.revision,
      scope: projectProposal.scope,
    };
    await workspace.skills.approveProposal(projectRef);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listProposals()).resolves.toMatchObject({ data: [] });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ name: projectProposal.name, scope: "project" }),
      ]),
    });
    await expect(
      readFile(
        join(workspaceRoot, ".scopeapp", "skills", projectProposal.name, "SKILL.md"),
        "utf8",
      ),
    ).resolves.toContain(projectProposal.instructions);
    await expect(workspace.skills.approveProposal(projectRef)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && errorType(error.data) === "invalid_params",
    );

    const userSession = await client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP user Skill proposal",
    });
    const userRun = await client.runs.start({
      sessionId: asSessionId(userSession.id),
      input: [{ type: "text", text: "E2E_SKILL_PROPOSAL_USER save this workflow." }],
    });
    await collectRunEvents(userRun.events);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    const userProposal = (await workspace.skills.listProposals()).data.find(
      (proposal) => proposal.name === "e2e-user-proposal",
    );
    if (!userProposal) throw new Error("user proposal disappeared before review");
    expect(userProposal).toMatchObject({
      scope: "user",
      origin: "requested",
      sourceSession: userSession.id,
    });
    await workspace.skills.rejectProposal({
      name: userProposal.name,
      revision: userProposal.revision,
      scope: userProposal.scope,
    });
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listProposals()).resolves.toMatchObject({ data: [] });
    expect(
      (await client.skills.listLibrary()).data.some((skill) => skill.name === userProposal.name),
    ).toBe(false);

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

    const projectRoot = join(root, "workspace-file-events");
    const workspaceRoot = join(projectRoot, "packages", "desktop");
    await mkdir(workspaceRoot, { recursive: true });
    await execFileAsync("git", ["init", "--quiet"], { cwd: projectRoot });
    await execFileAsync("git", ["config", "user.name", "Runtime HTTP E2E"], {
      cwd: projectRoot,
    });
    await execFileAsync("git", ["config", "user.email", "runtime-http-e2e@example.invalid"], {
      cwd: projectRoot,
    });
    await writeFile(join(workspaceRoot, "tracked.txt"), "before\n");
    await execFileAsync("git", ["add", "packages/desktop/tracked.txt"], { cwd: projectRoot });
    await execFileAsync("git", ["commit", "--quiet", "-m", "baseline"], {
      cwd: projectRoot,
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
    await execFileAsync("git", ["add", "packages/desktop/tracked.txt"], { cwd: projectRoot });

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

    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      {
        topics: ["files.changed", "knowledge.changed"],
        watches: [{ watchId: "knowledge-files", workspace: { path: workspaceRoot } }],
      },
      streamController.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const initialHome = await workspace.knowledge.get("home");
    const initialProject = await workspace.knowledge.get("projectRoot");
    const initialCwd = await workspace.knowledge.get("cwd");

    const savedHome = await workspace.knowledge.update({
      scope: "home",
      content: "home knowledge\n",
      expectedRevision: initialHome.revision,
    });
    await expect(nextRuntimeEvent(events, "knowledge.changed")).resolves.toMatchObject({
      type: "knowledge.changed",
    });
    const savedProject = await workspace.knowledge.update({
      scope: "projectRoot",
      content: "project-root knowledge\n",
      expectedRevision: initialProject.revision,
    });
    await expect(nextRuntimeEvent(events, "knowledge.changed")).resolves.toMatchObject({
      type: "knowledge.changed",
    });
    const savedCwd = await workspace.knowledge.update({
      scope: "cwd",
      content: "workspace knowledge\n",
      expectedRevision: initialCwd.revision,
    });
    await expect(nextRuntimeEvent(events, "knowledge.changed")).resolves.toMatchObject({
      type: "knowledge.changed",
    });
    expect(savedHome).toMatchObject({
      scope: "home",
      content: "home knowledge\n",
      revision: expect.any(String),
      updatedAt: expect.any(String),
    });

    await expect(workspace.knowledge.get("home")).resolves.toMatchObject({
      scope: "home",
      content: "home knowledge\n",
      revision: savedHome.revision,
    });
    await expect(workspace.knowledge.get("projectRoot")).resolves.toMatchObject({
      scope: "projectRoot",
      content: "project-root knowledge\n",
      revision: savedProject.revision,
    });
    await expect(workspace.knowledge.get("cwd")).resolves.toMatchObject({
      scope: "cwd",
      content: "workspace knowledge\n",
      revision: savedCwd.revision,
    });
    await expect(workspace.knowledge.list()).resolves.toMatchObject({
      data: [
        {
          scope: "home",
          content: "home knowledge\n",
          revision: savedHome.revision,
          updatedAt: expect.any(String),
        },
        {
          scope: "projectRoot",
          content: "project-root knowledge\n",
          revision: savedProject.revision,
          updatedAt: expect.any(String),
        },
        {
          scope: "cwd",
          content: "workspace knowledge\n",
          revision: savedCwd.revision,
          updatedAt: expect.any(String),
        },
      ],
    });
    await expect(readFile(join(projectRoot, "SCOPEAPP.md"), "utf8")).resolves.toBe(
      "project-root knowledge\n",
    );
    await expect(readFile(join(workspaceRoot, "SCOPEAPP.md"), "utf8")).resolves.toBe(
      "workspace knowledge\n",
    );

    await expect(
      workspace.knowledge.update({
        scope: "cwd",
        content: "stale overwrite",
        expectedRevision: initialCwd.revision,
      }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && errorType(error.data) === "revision_conflict",
    );

    const clearedHome = await workspace.knowledge.update({
      scope: "home",
      content: "",
      expectedRevision: savedHome.revision,
    });
    await nextRuntimeEvent(events, "knowledge.changed");
    const clearedProject = await workspace.knowledge.update({
      scope: "projectRoot",
      content: "",
      expectedRevision: savedProject.revision,
    });
    await nextRuntimeEvent(events, "knowledge.changed");
    const clearedCwd = await workspace.knowledge.update({
      scope: "cwd",
      content: "",
      expectedRevision: savedCwd.revision,
    });
    await nextRuntimeEvent(events, "knowledge.changed");
    await expect(workspace.knowledge.list()).resolves.toMatchObject({
      data: [
        { scope: "home", content: "", revision: clearedHome.revision },
        { scope: "projectRoot", content: "", revision: clearedProject.revision },
        { scope: "cwd", content: "", revision: clearedCwd.revision },
      ],
    });

    // External editors and sync processes bypass knowledge.update. The same
    // runtime stream must still invalidate every cascade scope, after which
    // the SDK's cold read observes the exact new file content.
    for (const change of [
      {
        scope: "home" as const,
        path: join(runtimeData, "SCOPEAPP.md"),
        content: "external home\n",
      },
      {
        scope: "projectRoot" as const,
        path: join(projectRoot, "SCOPEAPP.md"),
        content: "external project\n",
      },
      {
        scope: "cwd" as const,
        path: join(workspaceRoot, "SCOPEAPP.md"),
        content: "external cwd\n",
      },
    ]) {
      await writeFile(change.path, change.content);
      await expect(nextRuntimeEvent(events, "knowledge.changed")).resolves.toMatchObject({
        type: "knowledge.changed",
      });
      await expect(workspace.knowledge.get(change.scope)).resolves.toMatchObject({
        scope: change.scope,
        content: change.content,
      });
    }
    streamController.abort();
    await events.return?.();
  }, 30_000);

  it("confines knowledge symlinks to their scope and preserves the physical file", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-knowledge-boundary");
    const outside = join(root, "knowledge-outside.md");
    await mkdir(workspaceRoot);
    await writeFile(outside, "outside secret\n", { mode: 0o600 });
    const alias = join(workspaceRoot, "SCOPEAPP.md");
    await symlink(outside, alias);
    const workspace = client.workspace({ path: workspaceRoot });

    for (const request of [
      () => workspace.knowledge.get("cwd"),
      () => workspace.knowledge.list(),
      () =>
        workspace.knowledge.update({
          scope: "cwd",
          content: "must not escape\n",
          expectedRevision:
            "sha256:0000000000000000000000000000000000000000000000000000000000000000",
        }),
    ]) {
      await expect(request()).rejects.toSatisfy(
        (error: unknown) =>
          error instanceof RpcError && errorType(error.data) === "path_outside_root",
      );
    }
    await expect(readFile(outside, "utf8")).resolves.toBe("outside secret\n");

    await rm(alias);
    const physical = join(workspaceRoot, "private", "knowledge.md");
    await mkdir(join(workspaceRoot, "private"));
    await writeFile(physical, "before\n", { mode: 0o600 });
    await chmod(physical, 0o600);
    await symlink(join("private", "knowledge.md"), alias);

    const before = await workspace.knowledge.get("cwd");
    expect(before).toMatchObject({ scope: "cwd", content: "before\n" });
    const saved = await workspace.knowledge.update({
      scope: "cwd",
      content: "after\n",
      expectedRevision: before.revision,
    });
    expect(saved).toMatchObject({ scope: "cwd", content: "after\n" });
    expect((await lstat(alias)).isSymbolicLink()).toBe(true);
    await expect(readFile(physical, "utf8")).resolves.toBe("after\n");
    expect((await stat(physical)).mode & 0o777).toBe(0o600);
    await expect(workspace.knowledge.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ scope: "cwd", content: "after\n", revision: saved.revision }),
      ]),
    });
  }, 30_000);

  it("round-trips project and user agent memory through durable cold reads", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const workspaceRoot = join(root, "workspace-agent-memory");
    await mkdir(workspaceRoot);
    const workspace = client.workspace({ path: workspaceRoot });
    const streamController = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["agentMemory.changed", "sessions.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();
    const project = await workspace.agentMemory.add("project memory marker");
    await expect(nextRuntimeEvent(runtimeEvents, "agentMemory.changed")).resolves.toMatchObject({
      type: "agentMemory.changed",
    });
    expect(project).toMatchObject({
      scope: "project",
      content: "project memory marker",
      origin: "user",
      status: "active",
      pinned: false,
    });
    const duplicate = await workspace.agentMemory.add("project memory marker");
    expect(duplicate.id).toBe(project.id);

    // The duplicate is a successful cold read of the existing item, not a
    // mutation. A following Session mutation must therefore be the next queued
    // event; this also proves the stream does not rely on a timing-based absence
    // assertion.
    const orderingMarker = await client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "Agent memory duplicate ordering marker",
    });
    const next = await within(runtimeEvents.next(), "event after duplicate agent-memory add");
    expect(next).toMatchObject({
      done: false,
      value: { type: "sessions.changed", sessionIds: expect.arrayContaining([orderingMarker.id]) },
    });

    const user = await client.agentMemory.add({
      scope: "user",
      content: "user memory marker",
    });
    await expect(nextRuntimeEvent(runtimeEvents, "agentMemory.changed")).resolves.toMatchObject({
      type: "agentMemory.changed",
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
    await expect(nextRuntimeEvent(runtimeEvents, "agentMemory.changed")).resolves.toMatchObject({
      type: "agentMemory.changed",
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
    await expect(nextRuntimeEvent(runtimeEvents, "agentMemory.changed")).resolves.toMatchObject({
      type: "agentMemory.changed",
    });
    await client.agentMemory.delete(user.id);
    await expect(nextRuntimeEvent(runtimeEvents, "agentMemory.changed")).resolves.toMatchObject({
      type: "agentMemory.changed",
    });
    expect((await workspace.agentMemory.list()).items.some((item) => item.id === project.id)).toBe(
      false,
    );
    expect(
      (await client.agentMemory.list({ scope: "user" })).items.some((item) => item.id === user.id),
    ).toBe(false);
    streamController.abort();
    await runtimeEvents.return?.();
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

  it("consumes workspace files, recipes, agent docs and hook trust through bound APIs", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const projectRoot = join(root, "workspace-side-api-project");
    const workspaceRoot = join(projectRoot, "packages", "desktop");
    await mkdir(workspaceRoot, { recursive: true });
    await execFileAsync("git", ["init", "--quiet"], { cwd: projectRoot });

    const projectRecipe = join(workspaceRoot, ".scopeapp", "recipes", "project-side-api.md");
    const globalRecipe = join(runtimeData, "recipes", "global-side-api.md");
    await Promise.all([
      mkdir(join(workspaceRoot, ".scopeapp", "recipes"), { recursive: true }),
      mkdir(join(projectRoot, ".scopeapp"), { recursive: true }),
      mkdir(join(runtimeHome, ".scopeapp"), { recursive: true }),
      mkdir(join(runtimeData, "recipes"), { recursive: true }),
      mkdir(join(workspaceRoot, "nested"), { recursive: true }),
    ]);
    await Promise.all([
      writeFile(join(workspaceRoot, "alpha.txt"), "first\nworkspace-side-api-marker\nthird\n"),
      writeFile(join(workspaceRoot, "nested", "bravo.txt"), "workspace-side-api-marker\n"),
      writeFile(join(workspaceRoot, ".gitignore"), "ignored.log\n"),
      writeFile(join(workspaceRoot, "ignored.log"), "ignored marker\n"),
      writeFile(join(projectRoot, "AGENTS.md"), "project-root instructions\n"),
      writeFile(join(workspaceRoot, ".scopeapp", "AGENTS.md"), "workspace instructions\n"),
      writeFile(join(runtimeHome, ".scopeapp", "AGENTS.md"), "home instructions\n"),
      writeFile(
        projectRecipe,
        '---\ndescription: Project side API recipe\nargumentHint: "[target]"\n---\nReview $1\n',
      ),
      writeFile(
        globalRecipe,
        "---\ndescription: Global side API recipe\n---\nExplain $ARGUMENTS\n",
      ),
      writeFile(
        join(runtimeHome, ".scopeapp", "hooks.json"),
        JSON.stringify({ hooks: [{ event: "SessionStart", inject: "global hook context" }] }),
      ),
      writeFile(
        join(projectRoot, ".scopeapp", "hooks.json"),
        JSON.stringify({
          hooks: [{ event: "PreToolUse", matcher: "shell", command: "true" }],
        }),
      ),
    ]);

    const resolved = await client.workspaces.resolve({ path: workspaceRoot });
    const canonicalWorkspaceRoot = await realpath(workspaceRoot);
    const canonicalRuntimeHome = await realpath(runtimeHome);
    const canonicalRuntimeData = await realpath(runtimeData);
    expect(resolved).toMatchObject({
      ref: { path: canonicalWorkspaceRoot },
      projectRoot: await realpath(projectRoot),
      availability: "available",
    });
    if (!resolved.projectRoot) throw new Error("nested Git workspace omitted its project root");

    const workspace = client.workspace({ path: workspaceRoot });
    await expect(workspace.files.head({ path: "alpha.txt", lines: 2 })).resolves.toEqual({
      path: "alpha.txt",
      lines: [
        { lineNumber: 1, text: "first" },
        { lineNumber: 2, text: "workspace-side-api-marker" },
      ],
    });
    await expect(workspace.files.search({ query: "workspace-side-api-marker" })).resolves.toEqual({
      matches: [
        { path: "alpha.txt", lineNumber: 2, text: "workspace-side-api-marker" },
        { path: "nested/bravo.txt", lineNumber: 1, text: "workspace-side-api-marker" },
      ],
      total: 2,
    });

    const listed = await workspace.files.list({ recursive: true, limit: 1 }).autoPagingToArray();
    expect(listed).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "alpha.txt", name: "alpha.txt", type: "file" }),
        expect.objectContaining({ path: "nested/bravo.txt", name: "bravo.txt", type: "file" }),
      ]),
    );
    expect(listed.some((entry) => entry.path === "ignored.log")).toBe(false);
    const includingIgnored = await workspace.files
      .list({ recursive: true, includeIgnored: true, limit: 1 })
      .autoPagingToArray();
    expect(includingIgnored).toEqual(
      expect.arrayContaining([expect.objectContaining({ path: "ignored.log", type: "file" })]),
    );

    // A plain filesystem workspace must answer a lazy, one-level browser read
    // without walking all descendants first. Empty directories are real first-
    // level entries even though no flat candidate file can imply their presence.
    const plainRoot = join(root, "workspace-side-api-plain");
    await mkdir(join(plainRoot, "empty"), { recursive: true });
    await writeFile(join(plainRoot, ".git"), "gitdir: ../not-a-repository\n");
    await writeFile(join(plainRoot, "visible.txt"), "visible\n");
    const plainListing = await client.workspace({ path: plainRoot }).files.list();
    expect(plainListing).toMatchObject({
      data: [
        expect.objectContaining({ path: "empty", name: "empty", type: "dir" }),
        expect.objectContaining({ path: "visible.txt", name: "visible.txt", type: "file" }),
      ],
    });
    expect(plainListing.data.some((entry) => entry.path === ".git")).toBe(false);
    const outsidePlainRoot = join(root, "workspace-side-api-outside");
    await mkdir(outsidePlainRoot);
    await writeFile(join(outsidePlainRoot, "secret.txt"), "secret\n");
    await symlink(outsidePlainRoot, join(plainRoot, "escape"), "dir");
    await expect(
      client.workspace({ path: plainRoot }).files.list({ path: "escape" }),
    ).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && errorType(error.data) === "path_outside_root",
    );
    await expect(workspace.files.head({ path: "../AGENTS.md" })).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof RpcError && errorType(error.data) === "path_outside_root",
    );

    await expect(workspace.recipes.list()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({
          name: "project-side-api",
          description: "Project side API recipe",
          argumentHint: "[target]",
          body: "Review $1",
          scope: "project",
          source: join(canonicalWorkspaceRoot, ".scopeapp", "recipes", "project-side-api.md"),
        }),
        expect.objectContaining({
          name: "global-side-api",
          description: "Global side API recipe",
          body: "Explain $ARGUMENTS",
          scope: "global",
          source: join(canonicalRuntimeData, "recipes", "global-side-api.md"),
        }),
      ]),
    });
    await expect(workspace.agentDocs.list()).resolves.toEqual({
      data: [
        { path: join(canonicalRuntimeHome, ".scopeapp", "AGENTS.md"), scope: "home" },
        { path: join(resolved.projectRoot, "AGENTS.md"), scope: "projectRoot" },
        { path: join(canonicalWorkspaceRoot, ".scopeapp", "AGENTS.md"), scope: "cwd" },
      ],
    });

    const untrusted = await workspace.hooks.list();
    expect(untrusted).toMatchObject({
      projectRoot: resolved.projectRoot,
      projectTrusted: false,
    });
    expect(untrusted.hooks).toEqual([
      expect.objectContaining({
        event: "SessionStart",
        scope: "global",
        inject: "global hook context",
        active: true,
      }),
      expect.objectContaining({
        event: "PreToolUse",
        scope: "project",
        matcher: "shell",
        command: "true",
        active: false,
      }),
    ]);
    const hookController = new AbortController();
    const hookSubscription = await client.runtimeEvents.subscribe(
      {
        topics: ["files.changed", "hooks.changed"],
        watches: [{ watchId: "hook-files", workspace: { path: workspaceRoot } }],
      },
      hookController.signal,
    );
    const hookEvents = hookSubscription.events[Symbol.asyncIterator]();
    await client.hooks.setTrust(resolved.projectRoot, true);
    await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
      type: "hooks.changed",
    });
    await expect(workspace.hooks.list()).resolves.toMatchObject({
      projectRoot: resolved.projectRoot,
      projectTrusted: true,
      hooks: [
        expect.objectContaining({ scope: "global", active: true }),
        expect.objectContaining({ scope: "project", active: true }),
      ],
    });
    await client.hooks.setTrust(resolved.projectRoot, false);
    await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
      type: "hooks.changed",
    });
    await expect(workspace.hooks.list()).resolves.toMatchObject({ projectTrusted: false });

    // hooks.json has no mutation API; direct global/project/cwd edits are its
    // authoritative input and must converge through hooks.changed.
    const externalHookChanges = [
      {
        path: join(runtimeHome, ".scopeapp", "hooks.json"),
        marker: "external global hook",
        scope: "global",
        event: "SessionStart",
      },
      {
        path: join(projectRoot, ".scopeapp", "hooks.json"),
        marker: "external project hook",
        scope: "project",
        event: "UserPromptSubmit",
      },
      {
        path: join(workspaceRoot, ".scopeapp", "hooks.json"),
        marker: "external cwd hook",
        scope: "project",
        event: "Notification",
      },
    ] as const;
    for (const change of externalHookChanges) {
      await mkdir(dirname(change.path), { recursive: true });
      await writeFile(
        change.path,
        JSON.stringify({ hooks: [{ event: change.event, inject: change.marker }] }),
      );
      await expect(nextRuntimeEvent(hookEvents, "hooks.changed")).resolves.toMatchObject({
        type: "hooks.changed",
      });
      await expect(workspace.hooks.list()).resolves.toMatchObject({
        hooks: expect.arrayContaining([
          expect.objectContaining({ scope: change.scope, inject: change.marker }),
        ]),
      });
    }
    hookController.abort();
    await hookEvents.return?.();
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

  it("reopens durable reads and event reconciliation after a Runtime process restart", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const beforeRestart = await client.runtime.discover();
    const durable = await client.sessions.create({ title: "HTTP restart durability" });
    await client.close();
    client = undefined;
    await stopRuntimeProcess();

    await startRuntimeProcess();
    client = createRuntimeClient();

    const afterRestart = await client.runtime.discover();
    expect(afterRestart).toMatchObject({
      capabilities: {
        limits: {
          idempotency: {
            namespace: beforeRestart.capabilities.limits.idempotency.namespace,
          },
        },
      },
    });
    expect(afterRestart.serverInfo.instanceId).not.toBe(beforeRestart.serverInfo.instanceId);
    await expect(client.sessions.get(asSessionId(durable.id))).resolves.toMatchObject({
      id: durable.id,
      title: durable.title,
    });

    const controller = new AbortController();
    const subscription = await client.runtimeEvents.subscribe(
      { topics: ["sessions.changed"] },
      controller.signal,
    );
    const events = subscription.events[Symbol.asyncIterator]();
    const created = await client.sessions.create({ title: "HTTP restart event" });
    await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
      type: "sessions.changed",
      sessionIds: [created.id],
    });
    controller.abort();
    await events.return?.();
  }, 30_000);

  it("recovers durable HITL, Plan, Goal, Run and Tool reads after SIGKILL", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const beforeKill = await client.runtime.discover();
    const planSession = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP SIGKILL plan durability",
    });
    const planned = await client.runs.start({
      sessionId: asSessionId(planSession.id),
      input: [{ type: "text", text: "E2E_PLAN preserve this plan across SIGKILL." }],
    });
    await collectRunEvents(planned.events);

    const goalSession = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP SIGKILL Goal and HITL durability",
    });
    const goalSessionId = asSessionId(goalSession.id);
    await client.goals.start({
      sessionId: goalSessionId,
      objective: "E2E_HITL preserve this Goal wait across SIGKILL.",
      budget: { maxRuns: 1 },
    });
    let goal = await client.goals.get(goalSessionId);
    for (let attempt = 0; attempt < 100 && goal?.status === "active"; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      goal = await client.goals.get(goalSessionId);
    }
    expect(goal).toMatchObject({
      status: "paused",
      reason: { code: "awaitingInput" },
    });
    const goalRuns = await client.runs.list({ sessionId: goalSessionId });
    const waitingRun = goalRuns.data[0];
    if (!waitingRun) throw new Error("SIGKILL fixture omitted the Goal-owned Run");
    const waitingRunId = asRunId(waitingRun.id);
    const pending = await client.interrupts.list({ rootRunId: waitingRunId });
    const question = pending.data[0]?.interrupts[0];
    if (!question || question.type !== "question") {
      throw new Error("SIGKILL fixture omitted the Goal-owned HITL question");
    }

    const activeGoalSession = await client.sessions.create({
      workspace: { path: root },
      title: "HTTP SIGKILL active Goal recovery",
    });
    const activeGoalSessionId = asSessionId(activeGoalSession.id);
    const gate = createProviderGate("E2E_FORCE_KILL_GOAL");
    providerGate = gate;
    try {
      await client.goals.start({
        sessionId: activeGoalSessionId,
        objective: "E2E_FORCE_KILL_GOAL remain active until process death.",
        budget: { maxRuns: 2 },
      });
      await within(gate.arrived.promise, "the SIGKILL Goal model request");
      const activeGoalRuns = await client.runs.list({ sessionId: activeGoalSessionId });
      const activeGoalRun = activeGoalRuns.data.find((run) => run.status === "running");
      if (!activeGoalRun) throw new Error("SIGKILL fixture omitted the active Goal-owned Run");
      const activeGoalRunId = asRunId(activeGoalRun.id);

      await killRuntimeProcess();
      await within(gate.closed.promise, "the SIGKILL provider connection to close");
      gate.release.resolve();
      providerGate = undefined;
      await client.close();
      client = undefined;

      await startRuntimeProcess();
      client = createRuntimeClient();

      const afterKill = await client.runtime.discover();
      expect(afterKill).toMatchObject({
        capabilities: {
          limits: {
            idempotency: {
              namespace: beforeKill.capabilities.limits.idempotency.namespace,
            },
          },
        },
      });
      expect(afterKill.serverInfo.instanceId).not.toBe(beforeKill.serverInfo.instanceId);
      const planSnapshot = await client.sessions.snapshot(asSessionId(planSession.id));
      expect(planSnapshot).toMatchObject({
        plan: {
          revision: 1,
          steps: [
            { description: "Inspect the runtime contract", status: "completed" },
            { description: "Verify frontend reconciliation", status: "in_progress" },
          ],
        },
        items: expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "completed",
            tool: expect.objectContaining({ name: "set_plan" }),
          }),
        ]),
      });
      await expect(client.goals.get(goalSessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "awaitingInput" },
      });
      await expect(client.sessions.snapshot(goalSessionId)).resolves.toMatchObject({
        goal: {
          status: "paused",
          reason: { code: "awaitingInput" },
        },
        runs: [expect.objectContaining({ id: waitingRunId, status: "waiting" })],
        interrupts: [
          expect.objectContaining({
            interrupts: [expect.objectContaining({ itemId: question.itemId, type: "question" })],
          }),
        ],
      });
      await expect(client.runs.get(activeGoalRunId)).resolves.toMatchObject({
        status: "finished",
        outcome: { type: "lost", error: expect.objectContaining({ type: "run_lost" }) },
      });
      await expect(client.goals.get(activeGoalSessionId)).resolves.toMatchObject({
        status: "paused",
        reason: { code: "runNotCompleted", detail: "lost" },
        used: { runs: 1 },
      });
      await expect(client.sessions.snapshot(activeGoalSessionId)).resolves.toMatchObject({
        goal: {
          status: "paused",
          reason: { code: "runNotCompleted", detail: "lost" },
          used: { runs: 1 },
        },
        runs: expect.arrayContaining([
          expect.objectContaining({
            id: activeGoalRunId,
            status: "finished",
            outcome: expect.objectContaining({ type: "lost" }),
          }),
        ]),
      });
      const controller = new AbortController();
      const subscription = await client.runtimeEvents.subscribe(
        { topics: ["sessions.changed"] },
        controller.signal,
      );
      const events = subscription.events[Symbol.asyncIterator]();
      const created = await client.sessions.create({ title: "HTTP SIGKILL recovered event" });
      await expect(nextRuntimeEvent(events, "sessions.changed")).resolves.toMatchObject({
        type: "sessions.changed",
        sessionIds: [created.id],
      });
      controller.abort();
      await events.return?.();
    } finally {
      gate.release.resolve();
      providerGate = undefined;
    }
  }, 30_000);

  it("marks an in-flight Tool incomplete and its Run lost after SIGKILL", async () => {
    if (!client) throw new Error("runtime client was not initialized");

    const server = "http-e2e-kill";
    const toolCallMarker = join(environmentRoot, "sigkill-tool-call-arrived");
    const gate = createProviderGate("E2E_FORCE_KILL_TOOL", 1);
    let serverCreated = false;
    providerGate = gate;
    try {
      await client.mcp.create({
        autoApproveTools: ["ping"],
        connection: {
          type: "stdio",
          command: process.execPath,
          args: [mcpFixturePath, `--hold-tool-call=${toolCallMarker}`],
        },
        description: "SIGKILL Tool recovery fixture",
        enabled: true,
        name: server,
      });
      serverCreated = true;
      let connected = (await client.mcp.list()).data.find((entry) => entry.name === server);
      for (let attempt = 0; attempt < 100 && connected?.status.type !== "connected"; attempt++) {
        await new Promise((resolve) => setTimeout(resolve, 25));
        connected = (await client.mcp.list()).data.find((entry) => entry.name === server);
      }
      expect(connected).toMatchObject({ status: { type: "connected", toolCount: 1 } });

      const session = await client.sessions.create({
        workspace: { path: root },
        title: "HTTP SIGKILL active Tool recovery",
      });
      const started = await client.runs.start({
        sessionId: asSessionId(session.id),
        input: [{ type: "text", text: "E2E_FORCE_KILL_TOOL remain active until process death." }],
      });
      const runId = asRunId(started.result.runId);
      const drain = collectRunEvents(started.events).then(
        () => undefined,
        () => undefined,
      );
      await within(gate.arrived.promise, "the SIGKILL Tool model request");
      gate.release.resolve();
      providerGate = undefined;

      for (let attempt = 0; attempt < 200; attempt++) {
        if (
          await lstat(toolCallMarker).then(
            () => true,
            () => false,
          )
        )
          break;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      await expect(lstat(toolCallMarker)).resolves.toBeDefined();

      let items = await client.items.list({ scope: { type: "run", runId } });
      for (
        let attempt = 0;
        attempt < 200 &&
        !items.data.some(
          (item) =>
            item.type === "toolCall" &&
            item.status === "running" &&
            item.tool?.name === killedMCPToolName,
        );
        attempt++
      ) {
        await new Promise((resolve) => setTimeout(resolve, 25));
        items = await client.items.list({ scope: { type: "run", runId } });
      }
      expect(items.data).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "running",
            tool: expect.objectContaining({ name: killedMCPToolName }),
          }),
        ]),
      );

      await killRuntimeProcess();
      await within(drain, "the killed Tool Run stream to settle");
      await client.close();
      client = undefined;

      await startRuntimeProcess();
      client = createRuntimeClient();
      await expect(client.runs.get(runId)).resolves.toMatchObject({
        status: "finished",
        outcome: { type: "lost", error: expect.objectContaining({ type: "run_lost" }) },
      });
      await expect(client.items.list({ scope: { type: "run", runId } })).resolves.toMatchObject({
        data: expect.arrayContaining([
          expect.objectContaining({
            type: "toolCall",
            status: "incomplete",
            tool: expect.objectContaining({ name: killedMCPToolName }),
          }),
        ]),
      });
    } finally {
      gate.release.resolve();
      providerGate = undefined;
      if (serverCreated && client) await client.mcp.delete(server).catch(() => undefined);
    }
  }, 30_000);
});
