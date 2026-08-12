import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeServiceSnapshot } from "@/plugins/builtin/runtime/public/serviceStatus";
import { ConnectionPane } from "./ConnectionPane";

const runtime = vi.hoisted(() => ({
  refresh: vi.fn(),
  snapshot: null as RuntimeServiceSnapshot | null,
}));

vi.mock("@/plugins/builtin/runtime/public/endpoint", () => ({
  DEFAULT_RUNTIME_ENDPOINT: "http://127.0.0.1:17171",
  currentRuntimeEndpoint: () => "http://127.0.0.1:17171",
  applyRuntimeEndpoint: (endpoint: string) => ({
    kind: "accepted",
    endpoint,
    changed: false,
  }),
  resetRuntimeEndpoint: () => ({
    kind: "accepted",
    endpoint: "http://127.0.0.1:17171",
    changed: false,
  }),
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeServiceStatus: () => runtime.snapshot,
  refreshRuntimeServiceStatus: () => runtime.refresh(),
}));

describe("ConnectionPane runtime status", () => {
  beforeEach(() => {
    runtime.refresh.mockReset().mockResolvedValue(undefined);
  });

  it("renders degraded identity, protocol, and failing dependency checks", () => {
    runtime.snapshot = {
      phase: "degraded",
      failure: null,
      observation: {
        server: { name: "lyra-runtime", version: "1.2.3" },
        protocol: { current: "2026-07-01", minSupported: "2026-01-01" },
        health: "degraded",
        checks: { sqlite: "ready", git: "degraded" },
      },
    };

    render(<ConnectionPane />);

    expect(screen.getAllByText("Degraded")).toHaveLength(2);
    expect(screen.getByText(/lyra-runtime 1\.2\.3/)).toBeTruthy();
    expect(screen.getByText("2026-07-01")).toBeTruthy();
    expect(screen.getByText("git")).toBeTruthy();
    expect(screen.queryByText("sqlite")).toBeNull();
  });

  it("shows an unavailable detail and delegates a retry to Runtime", () => {
    runtime.snapshot = {
      phase: "unavailable",
      observation: null,
      failure: { reason: "failed", detail: "connection refused" },
    };

    render(<ConnectionPane />);

    expect(screen.getByText("Unavailable")).toBeTruthy();
    expect(screen.getByText("connection refused")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(runtime.refresh).toHaveBeenCalledOnce();
  });
});
