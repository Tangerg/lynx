import type { QueryFilters } from "@tanstack/react-query";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { SkillCurationGateway, SkillProposalHandle } from "./ports/skillCurationGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  type ManagedSkill,
  type SkillProposal,
  type WorkspaceCatalogQuery,
  type WorkspaceSkill,
} from "./workspaceQueries";

class SkillCurationRetiredError extends Error {
  override readonly name = "SkillCurationRetiredError";

  constructor() {
    super("skill_curation_generation_retired");
  }
}

interface SkillCurationCommand {
  execute(): Promise<void>;
  commit(): void;
  repair(): readonly QueryFilters[];
}

class SkillCurationGeneration {
  readonly #gateway: SkillCurationGateway;
  readonly #cohort = new RetirableTaskCohort(new SkillCurationRetiredError());
  readonly #tails = new Map<string, Promise<void>>();

  constructor(gateway: SkillCurationGateway) {
    this.#gateway = gateway;
  }

  archive(name: string): Promise<void> {
    return this.#run(userSkillIdentity(name), {
      execute: () => this.#gateway.archive(name),
      commit: () => commitManagedSkillLifecycle(name, "archived"),
      repair: libraryRepair,
    });
  }

  restore(name: string): Promise<void> {
    return this.#run(userSkillIdentity(name), {
      execute: () => this.#gateway.restore(name),
      commit: () => commitManagedSkillLifecycle(name, "active"),
      repair: libraryRepair,
    });
  }

  approveProposal(handle: SkillProposalHandle): Promise<void> {
    let binding: CachedProposalBinding | undefined;
    return this.#run(proposalIdentity(handle), {
      execute: () => {
        binding = cachedProposal(handle);
        return this.#gateway.approveProposal(handle);
      },
      commit: () => commitProposalDecision(handle, binding, true),
      repair: () => proposalRepair(handle, binding?.query, true),
    });
  }

  rejectProposal(handle: SkillProposalHandle): Promise<void> {
    let binding: CachedProposalBinding | undefined;
    return this.#run(proposalIdentity(handle), {
      execute: () => {
        binding = cachedProposal(handle);
        return this.#gateway.rejectProposal(handle);
      },
      commit: () => commitProposalDecision(handle, binding, false),
      repair: () => proposalRepair(handle, binding?.query, false),
    });
  }

  retire(): void {
    this.#cohort.retire();
    this.#tails.clear();
  }

  #run(identity: string, command: SkillCurationCommand): Promise<void> {
    const result = this.#settle(this.#tails.get(identity) ?? Promise.resolve()).then(async () => {
      this.#assertCurrent();
      try {
        await this.#settle(command.execute());
      } catch (error) {
        if (this.#cohort.retired) throw error;
        await this.#repair(command.repair());
        throw error;
      }
      this.#assertCurrent();
      command.commit();
      await this.#repair(command.repair());
      this.#assertCurrent();
    });
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#tails.get(identity) === settlement) this.#tails.delete(identity);
    });
    return result;
  }

  async #repair(filters: readonly QueryFilters[]): Promise<void> {
    try {
      await Promise.all(
        filters.map((filter) =>
          this.#settle(queryClient.invalidateQueries(filter)).then(() => undefined),
        ),
      );
    } catch (error) {
      if (this.#cohort.retired) throw error;
      // The accepted command already committed every fact it proved. Runtime
      // events and the next read retain the projection repair path.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns Skill library curation and proposal review for one exact Plugin Host
 * and Runtime generation. Both command families write the same Skill identity,
 * so they deliberately share one resource-partitioned tail. */
export class SkillCurationOwner {
  readonly #gateway: SkillCurationGateway;
  #generation: SkillCurationGeneration;
  #disposed = false;

  private constructor(gateway: SkillCurationGateway) {
    this.#gateway = gateway;
    this.#generation = new SkillCurationGeneration(gateway);
  }

  static install(gateway: SkillCurationGateway): SkillCurationOwner {
    const owner = new SkillCurationOwner(gateway);
    skillCurationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): SkillCurationOwner {
    const owner = skillCurationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Skill curation owner is not installed");
    return owner;
  }

  archive(name: string): Promise<void> {
    return this.#generation.archive(name);
  }

  restore(name: string): Promise<void> {
    return this.#generation.restore(name);
  }

  approveProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#generation.approveProposal(handle);
  }

  rejectProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#generation.rejectProposal(handle);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !skillCurationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new SkillCurationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    skillCurationPublication.withdraw(this);
  }
}

const skillCurationPublication = createPublicationSlot<SkillCurationOwner>();

export function archiveSkill(name: string): Promise<void> {
  return SkillCurationOwner.current().archive(name);
}

export function restoreSkill(name: string): Promise<void> {
  return SkillCurationOwner.current().restore(name);
}

export function approveSkillProposal(handle: SkillProposalHandle): Promise<void> {
  return SkillCurationOwner.current().approveProposal(handle);
}

export function rejectSkillProposal(handle: SkillProposalHandle): Promise<void> {
  return SkillCurationOwner.current().rejectProposal(handle);
}

export function skillCurationWasRetired(error: unknown): boolean {
  return error instanceof SkillCurationRetiredError;
}

function userSkillIdentity(name: string): string {
  return `user\u0000${name}`;
}

function proposalIdentity(handle: SkillProposalHandle): string {
  return handle.scope === "user"
    ? userSkillIdentity(handle.name)
    : `project\u0000${handle.workspace}\u0000${handle.name}`;
}

function libraryRepair(): QueryFilters[] {
  return [
    { queryKey: [WORKSPACE_MANAGED_SKILLS_KEY], exact: true },
    { queryKey: [WORKSPACE_SKILLS_KEY] },
  ];
}

function proposalRepair(
  handle: SkillProposalHandle,
  query: WorkspaceCatalogQuery | undefined,
  approved: boolean,
): QueryFilters[] {
  const filters: QueryFilters[] = [
    query
      ? { queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY, query], exact: true }
      : { queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] },
  ];
  if (!approved) return filters;
  if (handle.scope === "user") filters.push(...libraryRepair());
  else if (query) filters.push({ queryKey: [WORKSPACE_SKILLS_KEY, query], exact: true });
  else filters.push({ queryKey: [WORKSPACE_SKILLS_KEY] });
  return filters;
}

function commitManagedSkillLifecycle(name: string, lifecycle: ManagedSkill["lifecycle"]): void {
  let saved: ManagedSkill | undefined;
  queryClient.setQueryData<ManagedSkill[]>([WORKSPACE_MANAGED_SKILLS_KEY], (current) =>
    current?.map((skill) => {
      if (skill.name !== name) return skill;
      saved = { ...skill, lifecycle };
      return saved;
    }),
  );
  if (lifecycle === "archived") removeDiscoveredSkill(name, "user");
  else if (saved) upsertDiscoveredSkill({ name, description: saved.description, scope: "user" });
}

interface CachedProposalBinding {
  proposal: SkillProposal;
  query: WorkspaceCatalogQuery;
}

function cachedProposal(handle: SkillProposalHandle): CachedProposalBinding | undefined {
  for (const [queryKey, proposals] of queryClient.getQueriesData<SkillProposal[]>({
    queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY],
  })) {
    const proposal = proposals?.find((candidate) => proposalMatches(candidate, handle));
    const query = workspaceCatalogQuery(queryKey[1]);
    if (proposal && query) return { proposal, query };
  }
  return undefined;
}

function commitProposalDecision(
  handle: SkillProposalHandle,
  binding: CachedProposalBinding | undefined,
  approved: boolean,
): void {
  queryClient.setQueriesData<SkillProposal[]>(
    { queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] },
    (current) => current?.filter((candidate) => !proposalMatches(candidate, handle)),
  );
  const proposal = binding?.proposal;
  if (!approved || !proposal) return;
  const discovered = {
    name: proposal.name,
    description: proposal.description,
    scope: proposal.scope,
  } satisfies WorkspaceSkill;
  if (proposal.scope === "user") {
    queryClient.setQueryData<ManagedSkill[]>([WORKSPACE_MANAGED_SKILLS_KEY], (current) =>
      current
        ? upsertByName(current, {
            name: discovered.name,
            description: discovered.description,
            lifecycle: "active",
          })
        : current,
    );
    upsertDiscoveredSkill(discovered);
    return;
  }
  if (!binding) return;
  queryClient.setQueryData<WorkspaceSkill[]>([WORKSPACE_SKILLS_KEY, binding.query], (current) =>
    current ? upsertByName(current, discovered) : current,
  );
}

function workspaceCatalogQuery(value: unknown): WorkspaceCatalogQuery | undefined {
  if (typeof value !== "object" || value === null || !("cwd" in value)) return undefined;
  return typeof value.cwd === "string" || value.cwd === undefined ? { cwd: value.cwd } : undefined;
}

function proposalMatches(proposal: SkillProposal, handle: SkillProposalHandle): boolean {
  return (
    proposal.workspace === handle.workspace &&
    proposal.name === handle.name &&
    proposal.revision === handle.revision &&
    proposal.scope === handle.scope
  );
}

function removeDiscoveredSkill(name: string, scope: WorkspaceSkill["scope"]): void {
  queryClient.setQueriesData<WorkspaceSkill[]>({ queryKey: [WORKSPACE_SKILLS_KEY] }, (current) =>
    current?.filter((skill) => skill.name !== name || skill.scope !== scope),
  );
}

function upsertDiscoveredSkill(saved: WorkspaceSkill): void {
  queryClient.setQueriesData<WorkspaceSkill[]>({ queryKey: [WORKSPACE_SKILLS_KEY] }, (current) =>
    current ? upsertByName(current, saved) : current,
  );
}

function upsertByName<T extends { name: string }>(current: T[], saved: T): T[] {
  const found = current.some((item) => item.name === saved.name);
  return found
    ? current.map((item) => (item.name === saved.name ? saved : item))
    : [...current, saved];
}
