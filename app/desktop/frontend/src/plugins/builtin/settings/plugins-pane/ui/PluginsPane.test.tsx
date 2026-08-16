import { render, screen } from "@testing-library/react";
import type { Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { startKernel, stopKernel } from "@/plugins/sdk";
import { trackInstallation } from "@/plugins/sdk/kernel";
import { PluginsPane } from "./PluginsPane";

let host: Host | undefined;

afterEach(async () => {
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

describe("PluginsPane installation facts", () => {
  it("renders built-in and sideload origins from the active Host read model", async () => {
    host = await startKernel([]);
    trackInstallation(host, "lyra.builtin.example", { remove: vi.fn() });
    trackInstallation(host, "third.party", { remove: vi.fn() }, "sideload");

    const view = render(<PluginsPane />);

    expect(screen.getByText("lyra.builtin.example")).toBeTruthy();
    expect(screen.getByTitle("Ships with Lyra")).toBeTruthy();
    expect(screen.getByText("third.party")).toBeTruthy();
    expect(screen.getByTitle("User-installed")).toBeTruthy();
    view.unmount();
  });
});
