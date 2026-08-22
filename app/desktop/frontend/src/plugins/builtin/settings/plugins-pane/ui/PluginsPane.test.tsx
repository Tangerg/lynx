import { render, screen } from "@testing-library/react";
import type { Host } from "dougong";
import { afterEach, describe, expect, it } from "vitest";
import { startKernel, stopKernel } from "@/plugins/sdk";
import { trackInstalledPlugin } from "@/plugins/sdk/kernel";
import { PluginsPane } from "./PluginsPane";

let host: Host | undefined;

afterEach(async () => {
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

describe("PluginsPane installation facts", () => {
  it("renders installed plugins from the active Host read model", async () => {
    host = await startKernel([]);
    trackInstalledPlugin(host, "lyra.builtin.example");

    const view = render(<PluginsPane />);

    expect(screen.getByText("lyra.builtin.example")).toBeTruthy();
    view.unmount();
  });
});
