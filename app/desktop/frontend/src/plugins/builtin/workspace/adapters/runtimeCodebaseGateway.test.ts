import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { codebaseGateway } from "../application/ports/codebaseGateway";
import { installCodebaseGateway } from "./runtimeCodebaseGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

describe("runtimeCodebaseGateway", () => {
  it("preserves the Runtime operation identity for background reindex", async () => {
    const reindex = vi.fn().mockResolvedValue({ operationId: "op_1" });
    const open = vi.fn().mockResolvedValue({ codebase: { reindex } });
    setContainer({ client: () => ({ workspaces: { open } }) as unknown as LyraClient });
    uninstall = installCodebaseGateway();

    await expect(codebaseGateway().reindex("/repo")).resolves.toEqual({ operationId: "op_1" });
    expect(open).toHaveBeenCalledWith({ path: "/repo" });
  });
});
