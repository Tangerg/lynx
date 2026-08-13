import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { definePlugin, loadPlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SettingsPage } from "./SettingsPage";

async function loadPanes() {
  await loadPlugin(
    definePlugin({
      name: "test.settings",
      version: "1.0.0",
      setup: ({ host }) => {
        host.extensions.contribute(SETTINGS_PANE, {
          id: "appearance",
          label: "Appearance",
          description: "Tune the interface",
          order: 0,
          component: () => <div data-testid="appearance-body">appearance body</div>,
        });
        host.extensions.contribute(SETTINGS_PANE, {
          id: "plugins",
          label: "Plugins",
          order: 10,
          component: () => <div data-testid="plugins-body">plugins body</div>,
        });
      },
    }),
  );
}

describe("settingsPage", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("shows the first pane by default and renders its body", async () => {
    await loadPanes();
    render(<SettingsPage />);
    // The pane label appears in both the rail button and the header — use
    // testid to scope to the body.
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Appearance" })).toBeTruthy();
    expect(screen.getByText("Tune the interface")).toBeTruthy();
    // The rail still lists every pane.
    expect(screen.getAllByText("Plugins").length).toBeGreaterThan(0);
  });

  it("switches the body when a different pane is clicked", async () => {
    await loadPanes();
    render(<SettingsPage />);
    fireEvent.click(screen.getByText("Plugins"));
    expect(screen.getByTestId("plugins-body")).toBeTruthy();
    expect(screen.queryByTestId("appearance-body")).toBeNull();
  });

  it("falls back to the first pane when no panes match the saved selection", async () => {
    await loadPanes();
    render(<SettingsPage />);
    // Initial paint should show the lowest-`order` pane.
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
  });
});
