import { getContainer } from "@/main/container";
import { configureSkillLibraryGateway } from "../application/ports/skillLibraryGateway";
import type { SkillLibraryGateway } from "../application/ports/skillLibraryGateway";

const gateway: SkillLibraryGateway = {
  async archive(name) {
    await getContainer().client().workspace.skills.archive(name);
  },
  async restore(name) {
    await getContainer().client().workspace.skills.restore(name);
  },
};

export function installSkillLibraryGateway(): () => void {
  return configureSkillLibraryGateway(gateway);
}
