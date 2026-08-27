import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { setHookTrust } from "../application/hookTrust";
import { HOOKS_KEY } from "../application/hookQueries";
import { installHookTrustGateway } from "./runtimeHookTrustGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [HOOKS_KEY] });
});

describe("runtimeHookTrustGateway", () => {
  it("retires in-flight and queued trust intents before installing a successor", async () => {
    const retiredWrite = deferred();
    const setTrustRetired = vi.fn(() => retiredWrite.promise);
    const setTrustSuccessor = vi.fn().mockResolvedValue(undefined);
    setContainer({
      client: () => ({ hooks: { setTrust: setTrustRetired } }) as unknown as ScopeAppClient,
    });
    const retiredInstallation = installHookTrustGateway();

    const inFlight = rejected(setHookTrust("/repo", true));
    const queued = rejected(setHookTrust("/repo", false));
    await vi.waitFor(() => expect(setTrustRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ hooks: { setTrust: setTrustSuccessor } }) as unknown as ScopeAppClient,
    });
    const successorInstallation = installHookTrustGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    retiredWrite.resolve();

    await expect(inFlight).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });
    expect(setTrustSuccessor).not.toHaveBeenCalled();
  });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((settle) => {
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
