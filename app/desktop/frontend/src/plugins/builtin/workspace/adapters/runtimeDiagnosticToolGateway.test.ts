import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { invokeDiagnosticTool } from "../application/diagnosticTool";
import {
  installDiagnosticToolGateway,
  type DiagnosticToolGatewayInstallation,
} from "./runtimeDiagnosticToolGateway";

const installations: DiagnosticToolGatewayInstallation[] = [];

afterEach(() => {
  for (let index = installations.length - 1; index >= 0; index--) {
    installations[index]!.dispose();
  }
  installations.length = 0;
  resetContainer();
});

describe("runtimeDiagnosticToolGateway", () => {
  it("invokes a direct diagnostic Tool with the selected workspace", async () => {
    const invoke = vi.fn().mockResolvedValue({ matches: 2 });
    setContainer({
      client: () => ({ tools: { invoke } }) as unknown as LyraClient,
    });
    installations.push(installDiagnosticToolGateway());

    await expect(
      invokeDiagnosticTool({
        name: "grep",
        arguments: { query: "TODO" },
        cwd: "/work/alpha",
      }),
    ).resolves.toEqual({ matches: 2 });
    expect(invoke).toHaveBeenCalledWith({
      name: "grep",
      arguments: { query: "TODO" },
      workspace: { path: "/work/alpha" },
    });
  });

  it("cannot settle an old Host diagnostic invocation into its successor", async () => {
    const retired = deferred<unknown>();
    const invokeRetired = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ tools: { invoke: invokeRetired } }) as unknown as LyraClient,
    });
    installations.push(installDiagnosticToolGateway());

    const invocation = rejected(
      invokeDiagnosticTool({ name: "grep", arguments: { query: "old" }, cwd: "/work/alpha" }),
    );
    await vi.waitFor(() => expect(invokeRetired).toHaveBeenCalledOnce());

    const invokeSuccessor = vi.fn().mockResolvedValue({ matches: 3 });
    setContainer({
      client: () => ({ tools: { invoke: invokeSuccessor } }) as unknown as LyraClient,
    });
    installations.push(installDiagnosticToolGateway());

    retired.resolve({ matches: 1 });
    await expect(invocation).resolves.toMatchObject({
      message: "diagnostic_tool_generation_retired",
    });
    expect(invokeSuccessor).not.toHaveBeenCalled();
  });

  it("retires in-flight and queued invocations at a Runtime generation boundary", async () => {
    const retired = deferred<unknown>();
    const invoke = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce({ matches: 4 });
    setContainer({ client: () => ({ tools: { invoke } }) as unknown as LyraClient });
    const installation = installDiagnosticToolGateway();
    installations.push(installation);

    const inFlight = rejected(
      invokeDiagnosticTool({ name: "grep", arguments: { query: "one" }, cwd: "/work/alpha" }),
    );
    const queued = rejected(
      invokeDiagnosticTool({ name: "grep", arguments: { query: "two" }, cwd: "/work/alpha" }),
    );
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledOnce());

    installation.replaceRuntimeGeneration();
    await expect(inFlight).resolves.toMatchObject({
      message: "diagnostic_tool_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "diagnostic_tool_generation_retired",
    });
    expect(invoke).toHaveBeenCalledOnce();

    retired.resolve({ matches: 1 });
    await expect(
      invokeDiagnosticTool({ name: "grep", arguments: { query: "new" }, cwd: "/work/alpha" }),
    ).resolves.toEqual({ matches: 4 });
    expect(invoke).toHaveBeenCalledTimes(2);
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
