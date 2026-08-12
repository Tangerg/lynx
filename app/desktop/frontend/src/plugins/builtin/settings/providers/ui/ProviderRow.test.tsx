import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ProviderConfiguration } from "../application/providerConfig";
import { ProviderRow } from "./ProviderRow";

const hooks = vi.hoisted(() => ({
  update: vi.fn(),
  test: vi.fn(),
}));

vi.mock("../application/providerConfig", () => ({
  useUpdateProvider: () => hooks.update,
  useTestProvider: () => hooks.test,
}));

const provider = (overrides: Partial<ProviderConfiguration> = {}): ProviderConfiguration => ({
  id: "openai-compatible",
  baseUrl: "",
  apiKeyMasked: "",
  requiresBaseUrl: true,
  ...overrides,
});

describe("ProviderRow", () => {
  beforeEach(() => {
    hooks.update.mockReset();
    hooks.test.mockReset();
  });

  it("rebuilds its draft from the authoritative saved resource", async () => {
    const saved = provider({
      baseUrl: "http://127.0.0.1:19999/v1",
      apiKeyMasked: "p4****ey",
      keySource: "stored",
    });
    hooks.update.mockResolvedValue(saved);
    const view = render(<ProviderRow p={provider()} />);

    fireEvent.change(screen.getByLabelText(/openai-compatible API/i), {
      target: { value: "p49-dummy-key" },
    });
    fireEvent.change(screen.getByLabelText(/openai-compatible Base URL/i), {
      target: { value: "  http://127.0.0.1:19999/v1  " },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect((screen.getByLabelText(/openai-compatible Base URL/i) as HTMLInputElement).value).toBe(
        "http://127.0.0.1:19999/v1",
      );
    });
    expect((screen.getByLabelText(/openai-compatible API/i) as HTMLInputElement).value).toBe("");
    expect(hooks.update).toHaveBeenCalledWith({
      provider: "openai-compatible",
      apiKey: "p49-dummy-key",
      baseUrl: "http://127.0.0.1:19999/v1",
    });

    view.rerender(<ProviderRow p={saved} />);
    expect((screen.getByRole("button", { name: /save/i }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });
});
