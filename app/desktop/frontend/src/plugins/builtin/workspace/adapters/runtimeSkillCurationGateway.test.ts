import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import {
  approveSkillProposal,
  archiveSkill,
  rejectSkillProposal,
  restoreSkill,
} from "../application/skillCuration";
import {
  installSkillCurationGateway,
  type SkillCurationGatewayInstallation,
} from "./runtimeSkillCurationGateway";

let installation: SkillCurationGatewayInstallation | undefined;

afterEach(async () => {
  installation?.dispose();
  installation = undefined;
  await resetContainer();
});

describe("runtimeSkillCurationGateway", () => {
  it.each(["approveProposal", "rejectProposal"] as const)(
    "%ss against the workspace that supplied the reviewed proposal",
    async (decision) => {
      const approveProposal = vi.fn().mockResolvedValue(undefined);
      const rejectProposal = vi.fn().mockResolvedValue(undefined);
      const open = vi.fn().mockResolvedValue({
        skills: { approveProposal, rejectProposal },
      });
      setContainer({
        client: () =>
          ({
            skills: {},
            workspaces: { open },
          }) as unknown as LyraClient,
      });
      installation = installSkillCurationGateway();

      await (decision === "approveProposal" ? approveSkillProposal : rejectSkillProposal)({
        workspace: "/work/reviewed",
        name: "verify",
        revision: "rev_1",
        scope: "project",
      });

      expect(open).toHaveBeenCalledWith({ path: "/work/reviewed" });
      const expected = { name: "verify", revision: "rev_1", scope: "project" };
      expect(
        decision === "approveProposal" ? approveProposal : rejectProposal,
      ).toHaveBeenCalledWith(expected);
    },
  );

  it.each(["archive", "restore"] as const)(
    "maps library %s through the captured client",
    async (decision) => {
      const archive = vi.fn().mockResolvedValue(undefined);
      const restore = vi.fn().mockResolvedValue(undefined);
      setContainer({
        client: () => ({ skills: { archive, restore } }) as unknown as LyraClient,
      });
      installation = installSkillCurationGateway();

      await (decision === "archive" ? archiveSkill : restoreSkill)("verify");

      expect(decision === "archive" ? archive : restore).toHaveBeenCalledWith("verify");
    },
  );
});
