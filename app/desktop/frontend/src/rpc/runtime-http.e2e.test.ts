// @vitest-environment node

import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { createLyraClient, type LyraClient } from "./sdk";
import { RpcError } from "./errors";
import { asSessionId } from "./ids";
import { errorType } from "./types";
import { createSidecarClient } from "./sidecar";
import { createHttpTransport } from "./transports/http";
import { PROTOCOL_VERSION, type RuntimeEvent } from "./wire.generated";

const execFileAsync = promisify(execFile);
const runtimeDirectory = resolve(process.cwd(), "../../runtime");

async function unusedLoopbackPort(): Promise<number> {
  const server = createServer();
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

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK", () => {
  let root = "";
  let baseUrl = "";
  let runtime: ReturnType<typeof import("node:child_process").spawn> | undefined;
  let client: LyraClient | undefined;
  let processOutput = "";

  beforeAll(async () => {
    root = await mkdtemp(join(tmpdir(), "lyra-runtime-e2e-"));
    const executable = join(root, "lyra-e2e");
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
        HOME: root,
        LYRA_HOME: join(root, ".lyra"),
        LYRA_PROVIDER: "openai",
        LYRA_MODEL: "gpt-4.1-mini",
        LYRA_APIKEY: "e2e-placeholder-key",
        OPENAI_API_KEY: "e2e-placeholder-key",
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
    if (root) await rm(root, { recursive: true, force: true });
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
});
