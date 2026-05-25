import type React from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { PluginBoundary } from "./PluginBoundary";
import { usePluginErrorStore } from "./sdk";

function Boom(): React.ReactNode {
  throw new Error("kaboom");
}

describe("pluginBoundary", () => {
  it("renders children when nothing throws", () => {
    render(
      <PluginBoundary plugin="ok.plugin">
        <span>hello</span>
      </PluginBoundary>,
    );
    expect(screen.getByText("hello")).toBeTruthy();
  });

  it("catches a child render error and shows the default fallback", () => {
    // Silence React's expected error log so the suite output stays clean.
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PluginBoundary plugin="bad.plugin" label="Bad Plugin">
        <Boom />
      </PluginBoundary>,
    );
    expect(screen.getByText("Bad Plugin")).toBeTruthy();
    expect(screen.getByText(/failed to render/i)).toBeTruthy();
    spy.mockRestore();
  });

  it("reports the error through the plugin error store with source=render", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <PluginBoundary plugin="bad.plugin">
        <Boom />
      </PluginBoundary>,
    );
    const log = usePluginErrorStore.getState().log;
    expect(log).toHaveLength(1);
    expect(log[0].plugin).toBe("bad.plugin");
    expect(log[0].source).toBe("render");
    expect(log[0].message).toBe("kaboom");
    spy.mockRestore();
  });
});
