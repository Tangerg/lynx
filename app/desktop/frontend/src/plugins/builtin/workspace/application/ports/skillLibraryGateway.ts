import { createSingletonPort } from "@/lib/ports/singletonPort";

// SkillLibraryGateway mutates the global self-authored skill library: archive a
// skill (remove from active use without deleting) or restore an archived one.
// The runtime adapter drives skills.library.* over RPC.
export interface SkillLibraryGateway {
  archive(name: string): Promise<void>;
  restore(name: string): Promise<void>;
}

const port = createSingletonPort<SkillLibraryGateway>("Skill library gateway is not configured");

export const configureSkillLibraryGateway = port.configure;
export const skillLibraryGateway = port.get;
