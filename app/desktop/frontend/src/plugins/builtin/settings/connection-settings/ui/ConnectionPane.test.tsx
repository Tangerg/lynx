import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeServiceSnapshot } from "@/plugins/builtin/runtime/public/serviceStatus";
import { ConnectionPane } from "./ConnectionPane";

const runtime = vi.hoisted(() => ({
  applyEndpoint: vi.fn(),
  refresh: vi.fn(),
  resetEndpoint: vi.fn(),
  snapshot: null as RuntimeServiceSnapshot | null,
}));

vi.mock("@/plugins/builtin/runtime/public/endpoint", () => ({
  DEFAULT_RUNTIME_ENDPOINT: "http://127.0.0.1:17171",
  currentRuntimeEndpoint: () => "http://127.0.0.1:17171",
  applyRuntimeEndpoint: runtime.applyEndpoint,
  resetRuntimeEndpoint: runtime.resetEndpoint,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  useRuntimeServiceStatus: () => runtime.snapshot,
  refreshRuntimeServiceStatus: () => runtime.refresh(),
}));

describe("ConnectionPane runtime status", () => {
  beforeEach(() => {
    runtime.applyEndpoint.mockReset().mockImplementation((endpoint: string) => ({
      kind: "applied",
      endpoint,
      changed: false,
    }));
    runtime.refresh.mockReset().mockResolvedValue(undefined);
    runtime.resetEndpoint.mockReset().mockReturnValue({
      kind: "applied",
      endpoint: "http://127.0.0.1:17171",
      changed: false,
    });
  });

  it("hands an endpoint replacement to the Runtime owner without reloading the renderer", () => {
    runtime.snapshot = {
      phase: "ready",
      failure: null,
      observation: {
        server: { name: "lyra-runtime", version: "1.2.3" },
        protocol: { current: "2026-07-01", minSupported: "2026-01-01" },
        health: "ready",
        checks: {},
      },
    };
    runtime.applyEndpoint.mockReturnValue({
      kind: "applied",
      endpoint: "http://127.0.0.1:27171",
      changed: true,
    });
    const reload = vi.spyOn(window.location, "reload").mockImplementation(() => undefined);
    render(<ConnectionPane />);

    fireEvent.change(screen.getByRole("textbox", { name: "URL" }), {
      target: { value: "http://127.0.0.1:27171" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Apply" }));

    expect(runtime.applyEndpoint).toHaveBeenCalledWith("http://127.0.0.1:27171");
    expect(reload).not.toHaveBeenCalled();
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
