import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import type { MessageActionMaterialization } from "@/plugins/builtin/chat/message-actions/public/messageActions";

type VisibleMaterialToken = symbol;
type VisibleMaterialGeneration = object;

interface VisibleProjection {
  generation: VisibleMaterialGeneration;
  settled: boolean;
}

/**
 * Owns the presentation generation of one exact transcript message.
 *
 * Runtime completion and DOM completion are different facts while the smooth
 * Markdown reveal drains its accepted source backlog. Terminal message actions
 * belong to their intersection: they cannot exist while either the durable
 * block or any mounted visible-text projection can still grow. A settled
 * projection belongs to one accepted transcript generation; its result cannot
 * be lent to a later terminal update before that update reaches the screen.
 */
export class MessageVisibleMaterialOwner {
  readonly identity: string;
  readonly #projections = new Map<VisibleMaterialToken, VisibleProjection>();
  readonly #listeners = new Set<() => void>();
  #revision = 0;

  constructor(sessionId: string, messageId: string) {
    this.identity = JSON.stringify([sessionId, messageId]);
  }

  observe(
    token: VisibleMaterialToken,
    generation: VisibleMaterialGeneration,
    settled: boolean,
  ): void {
    const current = this.#projections.get(token);
    if (current?.generation === generation && current.settled === settled) return;
    const wasActive = this.#projectionsOwnActiveMaterial(generation);
    this.#projections.set(token, { generation, settled });
    if (wasActive !== this.#projectionsOwnActiveMaterial(generation)) this.#publish();
  }

  retire(token: VisibleMaterialToken): void {
    if (!this.#projections.delete(token)) return;
    this.#publish();
  }

  actionsMaterialization(
    source: MessageActionMaterialization,
    generation: VisibleMaterialGeneration,
  ): MessageActionMaterialization {
    return source === "active" || this.#projectionsOwnActiveMaterial(generation)
      ? "active"
      : "settled";
  }

  #projectionsOwnActiveMaterial(generation: VisibleMaterialGeneration): boolean {
    for (const projection of this.#projections.values()) {
      if (projection.generation !== generation || !projection.settled) return true;
    }
    return false;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  snapshot = (): number => this.#revision;

  #publish(): void {
    this.#revision += 1;
    for (const listener of this.#listeners) listener();
  }
}

interface MessageVisibleMaterialContextValue {
  owner: MessageVisibleMaterialOwner;
  generation: VisibleMaterialGeneration;
}

const MessageVisibleMaterialContext = createContext<MessageVisibleMaterialContextValue | null>(
  null,
);

export function MessageVisibleMaterialProvider({
  owner,
  generation,
  children,
}: {
  owner: MessageVisibleMaterialOwner;
  generation: VisibleMaterialGeneration;
  children: ReactNode;
}) {
  const material = useMemo(() => ({ owner, generation }), [generation, owner]);
  return (
    <MessageVisibleMaterialContext.Provider value={material}>
      {children}
    </MessageVisibleMaterialContext.Provider>
  );
}

/** Re-render message chrome only when a visible projection crosses the
 * presenting/settled boundary. Active transcript rows share a stable generation,
 * so token-by-token text growth stays local; exact row identity matters once
 * durable material would otherwise admit terminal controls. */
export function useVisibleActionMaterialization(
  owner: MessageVisibleMaterialOwner,
  source: MessageActionMaterialization,
  generation: VisibleMaterialGeneration,
): MessageActionMaterialization {
  useSyncExternalStore(owner.subscribe, owner.snapshot, owner.snapshot);
  return owner.actionsMaterialization(source, generation);
}

/** Bind one mounted visible-text projection to the exact message owner. */
export function useVisibleTextMaterial(settled: boolean): void {
  const material = useContext(MessageVisibleMaterialContext);
  const [token] = useState<VisibleMaterialToken>(() => Symbol("visible-text-material"));

  useLayoutEffect(() => {
    if (!material) return;
    material.owner.observe(token, material.generation, settled);
  }, [material, settled, token]);

  const owner = material?.owner;
  useLayoutEffect(() => () => owner?.retire(token), [owner, token]);
}
