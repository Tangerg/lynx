import type { QueryFilters } from "@tanstack/react-query";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import {
  WorkspaceKnowledgeRevisionConflictError,
  type WorkspaceKnowledgeDocument,
  type WorkspaceKnowledgeGateway,
  type WorkspaceKnowledgeReadInput,
  type WorkspaceKnowledgeUpdateInput,
} from "./ports/knowledgeGateway";
import {
  WORKSPACE_KNOWLEDGE_KEY,
  type WorkspaceKnowledgeEntry,
  type WorkspaceKnowledgeQuery,
} from "./workspaceQueries";

class WorkspaceKnowledgeGenerationRetiredError extends Error {
  override readonly name = "WorkspaceKnowledgeGenerationRetiredError";

  constructor() {
    super("workspace_knowledge_generation_retired");
  }
}

class KnowledgeGeneration {
  readonly #gateway: WorkspaceKnowledgeGateway;
  readonly #cohort = new RetirableTaskCohort(new WorkspaceKnowledgeGenerationRetiredError());
  readonly #saveTails = new Map<string, Promise<void>>();

  constructor(gateway: WorkspaceKnowledgeGateway) {
    this.#gateway = gateway;
  }

  async read(input: WorkspaceKnowledgeReadInput): Promise<WorkspaceKnowledgeDocument> {
    const snapshot = await this.#settle(this.#gateway.read(input));
    this.#assertCurrent();
    return snapshot;
  }

  save(input: WorkspaceKnowledgeUpdateInput): Promise<WorkspaceKnowledgeDocument> {
    const identity = knowledgeIdentity(input);
    const result = this.#settle(this.#saveTails.get(identity) ?? Promise.resolve()).then(
      async () => {
        this.#assertCurrent();
        let saved: WorkspaceKnowledgeDocument;
        try {
          saved = await this.#settle(this.#gateway.save(input));
        } catch (error) {
          if (this.#cohort.retired) throw error;
          await this.#repair(knowledgeRepair(input));
          throw error;
        }
        this.#assertCurrent();
        commitKnowledgeDocument(input, saved);
        await this.#repair(knowledgeRepair(input));
        this.#assertCurrent();
        return saved;
      },
    );
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#saveTails.set(identity, settlement);
    void settlement.then(() => {
      if (this.#saveTails.get(identity) === settlement) this.#saveTails.delete(identity);
    });
    return result;
  }

  retire(): void {
    this.#cohort.retire();
    this.#saveTails.clear();
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
      // Accepted writes have already committed every fact proved by their
      // response. Runtime events and a later read retain the repair path.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns direct Knowledge reads and CAS writes for one exact Plugin Host and
 * Runtime generation. Local drafts remain UI material; only this owner may
 * settle Runtime commands or project their authoritative document facts. */
export class KnowledgeOwner {
  readonly #gateway: WorkspaceKnowledgeGateway;
  #generation: KnowledgeGeneration;
  #disposed = false;

  private constructor(gateway: WorkspaceKnowledgeGateway) {
    this.#gateway = gateway;
    this.#generation = new KnowledgeGeneration(gateway);
  }

  static install(gateway: WorkspaceKnowledgeGateway): KnowledgeOwner {
    const owner = new KnowledgeOwner(gateway);
    knowledgePublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): KnowledgeOwner {
    const owner = knowledgePublication.current();
    if (!owner || owner.#disposed) throw new Error("Workspace Knowledge owner is not installed");
    return owner;
  }

  read(input: WorkspaceKnowledgeReadInput): Promise<WorkspaceKnowledgeDocument> {
    return this.#generation.read(input);
  }

  save(input: WorkspaceKnowledgeUpdateInput): Promise<WorkspaceKnowledgeDocument> {
    return this.#generation.save(input);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !knowledgePublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new KnowledgeGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    knowledgePublication.withdraw(this);
  }
}

const knowledgePublication = createPublicationSlot<KnowledgeOwner>();

/** Immutable editor material for one exact Knowledge document. */
export class KnowledgeDraft {
  private constructor(
    readonly content: string,
    readonly draft: string,
    readonly revision: string,
    readonly updatedAt?: string,
  ) {}

  static open(snapshot: WorkspaceKnowledgeDocument): KnowledgeDraft {
    return new KnowledgeDraft(
      snapshot.content,
      snapshot.content,
      snapshot.revision,
      snapshot.updatedAt,
    );
  }

  get dirty(): boolean {
    return this.draft !== this.content;
  }

  edit(draft: string): KnowledgeDraft {
    return new KnowledgeDraft(this.content, draft, this.revision, this.updatedAt);
  }

  revert(): KnowledgeDraft {
    return this.edit(this.content);
  }

  reconcile(snapshot: WorkspaceKnowledgeDocument): KnowledgeDraft {
    if (this.revision === snapshot.revision || this.dirty) return this;
    return KnowledgeDraft.open(snapshot);
  }

  settleSave(
    saved: WorkspaceKnowledgeDocument,
    latest: WorkspaceKnowledgeDocument,
  ): KnowledgeDraft {
    const committed = new KnowledgeDraft(
      saved.content,
      this.draft,
      saved.revision,
      saved.updatedAt,
    );
    if (latest.revision === this.revision || latest.revision === saved.revision) return committed;
    return committed.reconcile(latest);
  }

  rebase(snapshot: WorkspaceKnowledgeDocument): KnowledgeDraft {
    return new KnowledgeDraft(snapshot.content, this.draft, snapshot.revision, snapshot.updatedAt);
  }
}

export function loadWorkspaceKnowledge(
  input: WorkspaceKnowledgeReadInput,
): Promise<WorkspaceKnowledgeDocument> {
  return KnowledgeOwner.current().read(input);
}

export function saveWorkspaceKnowledge(
  input: WorkspaceKnowledgeUpdateInput,
): Promise<WorkspaceKnowledgeDocument> {
  return KnowledgeOwner.current().save(input);
}

export function isWorkspaceKnowledgeRevisionConflict(
  error: unknown,
): error is WorkspaceKnowledgeRevisionConflictError {
  return error instanceof WorkspaceKnowledgeRevisionConflictError;
}

export function workspaceKnowledgeWasRetired(error: unknown): boolean {
  return error instanceof WorkspaceKnowledgeGenerationRetiredError;
}

function knowledgeIdentity(input: WorkspaceKnowledgeReadInput): string {
  if (input.scope === "home") return "home";
  return `${input.cwd ?? ""}\u0000${input.scope}`;
}

function knowledgeQuery(cwd: string | undefined): WorkspaceKnowledgeQuery {
  return { cwd };
}

function knowledgeRepair(input: WorkspaceKnowledgeReadInput): QueryFilters[] {
  if (input.scope !== "cwd") return [{ queryKey: [WORKSPACE_KNOWLEDGE_KEY] }];
  return [
    {
      queryKey: [WORKSPACE_KNOWLEDGE_KEY, knowledgeQuery(input.cwd)],
      exact: true,
    },
  ];
}

function commitKnowledgeDocument(
  input: WorkspaceKnowledgeUpdateInput,
  saved: WorkspaceKnowledgeDocument,
): void {
  const commit = (current: WorkspaceKnowledgeEntry[] | undefined) => {
    if (!current) return current;
    const entry = {
      scope: input.scope,
      content: saved.content,
      revision: saved.revision,
      ...(saved.updatedAt ? { updatedAt: saved.updatedAt } : {}),
    } satisfies WorkspaceKnowledgeEntry;
    const found = current.some((candidate) => candidate.scope === input.scope);
    return found
      ? current.map((candidate) => (candidate.scope === input.scope ? entry : candidate))
      : [...current, entry];
  };
  if (input.scope === "home") {
    queryClient.setQueriesData<WorkspaceKnowledgeEntry[]>(
      { queryKey: [WORKSPACE_KNOWLEDGE_KEY] },
      commit,
    );
    return;
  }
  queryClient.setQueryData<WorkspaceKnowledgeEntry[]>(
    [WORKSPACE_KNOWLEDGE_KEY, knowledgeQuery(input.cwd)],
    commit,
  );
}
