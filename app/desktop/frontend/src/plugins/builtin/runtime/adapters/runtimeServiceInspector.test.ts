import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { HTTP_ENDPOINTS, PROTOCOL_VERSION, type SidecarClient } from "@/rpc";
import { runtimeServiceInspector } from "./runtimeServiceInspector";

function sidecar(overrides: Partial<SidecarClient> = {}): SidecarClient {
  return {
    info: vi.fn().mockResolvedValue({
      protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
      server: { name: "lyra", version: "1.2.3" },
      transport: "http",
      endpoints: {
        rpc: HTTP_ENDPOINTS.rpc.path,
        info: HTTP_ENDPOINTS.info.path,
        liveness: HTTP_ENDPOINTS.liveness.path,
        readiness: HTTP_ENDPOINTS.readiness.path,
      },
    }),
    liveness: vi.fn().mockResolvedValue({ status: "ok" }),
    readiness: vi
      .fn()
      .mockResolvedValue({ status: "degraded", checks: { database: "ok", git: "degraded" } }),
    ...overrides,
  };
}

afterEach(resetContainer);

describe("runtime service inspector", () => {
  it("consumes all sidecars and removes their HTTP representation", async () => {
    const client = sidecar();
    setContainer({ sidecar: () => client });
    const signal = new AbortController().signal;

    await expect(runtimeServiceInspector().inspect(signal)).resolves.toEqual({
      server: { name: "lyra", version: "1.2.3" },
      protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
      health: "degraded",
      checks: { database: "ready", git: "degraded" },
    });
    const infoSignal = vi.mocked(client.info).mock.calls[0]?.[0];
    expect(infoSignal).toBeInstanceOf(AbortSignal);
    expect(infoSignal?.aborted).toBe(false);
    expect(client.liveness).toHaveBeenCalledWith(infoSignal);
    expect(client.readiness).toHaveBeenCalledWith(infoSignal);
  });

  it("refuses a server whose advertised binding disagrees with the compiled contract", async () => {
    const client = sidecar({
      info: vi.fn().mockResolvedValue({
        protocol: { current: PROTOCOL_VERSION, minSupported: PROTOCOL_VERSION },
        server: { name: "lyra", version: "1.2.3" },
        transport: "http",
        endpoints: {
          rpc: "/v3/rpc",
          info: HTTP_ENDPOINTS.info.path,
          liveness: HTTP_ENDPOINTS.liveness.path,
          readiness: HTTP_ENDPOINTS.readiness.path,
        },
      }),
    });
    setContainer({ sidecar: () => client });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "incompatible rpc endpoint",
    );
  });

  it("cancels sibling sidecars when one member of the inspection fails", async () => {
    let infoSignal: AbortSignal | undefined;
    const client = sidecar({
      info: vi.fn<SidecarClient["info"]>((signal) => {
        infoSignal = signal;
        return new Promise((_resolve, reject) => {
          signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        });
      }),
      readiness: vi.fn().mockRejectedValue(new Error("readiness failed")),
    });
    setContainer({ sidecar: () => client });

    await expect(runtimeServiceInspector().inspect(new AbortController().signal)).rejects.toThrow(
      "readiness failed",
    );
    expect(infoSignal?.aborted).toBe(true);
  });
});
