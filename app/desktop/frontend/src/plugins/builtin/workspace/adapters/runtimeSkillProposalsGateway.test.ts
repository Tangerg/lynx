import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { skillProposalsGateway } from "../application/ports/skillProposalsGateway";
import { installSkillProposalsGateway } from "./runtimeSkillProposalsGateway";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
});

describe("runtimeSkillProposalsGateway", () => {
  it.each(["approve", "reject"] as const)(
    "%ss against the workspace that supplied the reviewed proposal",
    async (decision) => {
      const approveProposal = vi.fn().mockResolvedValue(undefined);
      const rejectProposal = vi.fn().mockResolvedValue(undefined);
      const open = vi.fn().mockResolvedValue({
        skills: { approveProposal, rejectProposal },
      });
      setContainer({
        client: () => ({ workspaces: { open } }) as unknown as LyraClient,
      });
      uninstall = installSkillProposalsGateway();

      await skillProposalsGateway()[decision]({
        workspace: "/work/reviewed",
        name: "verify",
        revision: "rev_1",
        scope: "project",
      });

      expect(open).toHaveBeenCalledWith({ path: "/work/reviewed" });
      const expected = { name: "verify", revision: "rev_1", scope: "project" };
      expect(decision === "approve" ? approveProposal : rejectProposal).toHaveBeenCalledWith(
        expected,
      );
    },
  );
});
