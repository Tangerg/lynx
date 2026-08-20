import { createPublicationSlot } from "@/lib/publicationSlot";

export type GoalComposerModePhase = "inactive" | "armed" | "confirming" | "starting";

export interface GoalComposerModeSnapshot {
  sessionId: string | null;
  phase: GoalComposerModePhase;
  replacedObjective: string | null;
}

const INACTIVE: GoalComposerModeSnapshot = {
  sessionId: null,
  phase: "inactive",
  replacedObjective: null,
};

const publication = createPublicationSlot<GoalComposerModeOwner>();

/** Owns the one transient Goal execution mode attached to the one composer. */
export class GoalComposerModeOwner {
  #snapshot: GoalComposerModeSnapshot = INACTIVE;
  #replacement: (() => void) | null = null;
  #listeners = new Set<() => void>();
  #disposed = false;

  static install(): GoalComposerModeOwner {
    const owner = new GoalComposerModeOwner();
    publication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): GoalComposerModeOwner {
    const owner = publication.current();
    if (!owner || owner.#disposed) throw new Error("Goal composer mode owner is not installed");
    return owner;
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  readonly snapshot = (): GoalComposerModeSnapshot => this.#snapshot;

  ownsPublication(): boolean {
    return !this.#disposed && publication.owns(this);
  }

  active(sessionId: string): boolean {
    return this.#snapshot.sessionId === sessionId && this.#snapshot.phase !== "inactive";
  }

  activate(sessionId: string): boolean {
    if (!this.ownsPublication()) return false;
    this.#replacement = null;
    this.#publish({ sessionId, phase: "armed", replacedObjective: null });
    return true;
  }

  toggle(sessionId: string): boolean {
    if (!this.ownsPublication()) return false;
    if (this.#snapshot.sessionId === sessionId && this.#snapshot.phase === "armed") {
      return this.deactivate(sessionId);
    }
    if (this.#snapshot.phase === "confirming" || this.#snapshot.phase === "starting") return false;
    return this.activate(sessionId);
  }

  begin(sessionId: string): boolean {
    if (!this.ownsPublication() || !this.active(sessionId) || this.#snapshot.phase !== "armed") {
      return false;
    }
    this.#publish({ sessionId, phase: "starting", replacedObjective: null });
    return true;
  }

  requestReplacement(sessionId: string, objective: string, start: () => void): boolean {
    if (!this.ownsPublication() || !this.active(sessionId) || this.#snapshot.phase !== "armed") {
      return false;
    }
    this.#replacement = start;
    this.#publish({ sessionId, phase: "confirming", replacedObjective: objective });
    return true;
  }

  confirmReplacement(sessionId: string): boolean {
    if (
      !this.ownsPublication() ||
      this.#snapshot.sessionId !== sessionId ||
      this.#snapshot.phase !== "confirming" ||
      !this.#replacement
    ) {
      return false;
    }
    const start = this.#replacement;
    this.#replacement = null;
    this.#publish({ sessionId, phase: "starting", replacedObjective: null });
    start();
    return true;
  }

  cancelReplacement(sessionId: string): boolean {
    if (
      !this.ownsPublication() ||
      this.#snapshot.sessionId !== sessionId ||
      this.#snapshot.phase !== "confirming"
    ) {
      return false;
    }
    this.#replacement = null;
    this.#publish({ sessionId, phase: "armed", replacedObjective: null });
    return true;
  }

  finish(sessionId: string, succeeded: boolean): boolean {
    if (
      !this.ownsPublication() ||
      this.#snapshot.sessionId !== sessionId ||
      this.#snapshot.phase !== "starting"
    ) {
      return false;
    }
    this.#publish(succeeded ? INACTIVE : { sessionId, phase: "armed", replacedObjective: null });
    return true;
  }

  deactivate(sessionId: string): boolean {
    if (!this.ownsPublication() || this.#snapshot.sessionId !== sessionId) return false;
    this.#replacement = null;
    this.#publish(INACTIVE);
    return true;
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#replacement = null;
    this.#snapshot = INACTIVE;
    for (const listener of this.#listeners) listener();
    this.#listeners.clear();
    publication.withdraw(this);
  }

  #publish(snapshot: GoalComposerModeSnapshot): void {
    if (this.#snapshot === snapshot) return;
    this.#snapshot = snapshot;
    for (const listener of this.#listeners) listener();
  }
}
