import {
  createContext,
  useContext,
  useLayoutEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import type { MessageActionMaterialization } from "@/plugins/builtin/chat/message-actions/public/messageActions";

type VisibleMaterialToken = symbol;

/**
 * Owns the presentation generation of one exact transcript message.
 *
 * Runtime completion and DOM completion are different facts while the smooth
 * Markdown reveal drains its accepted source backlog. Terminal message actions
 * belong to their intersection: they cannot exist while either the durable
 * block or any mounted visible-text projection can still grow.
 */
export class MessageVisibleMaterialOwner {
  readonly identity: string;
  readonly #presenting = new Set<VisibleMaterialToken>();
  readonly #listeners = new Set<() => void>();
  #revision = 0;

  constructor(sessionId: string, messageId: string) {
    this.identity = JSON.stringify([sessionId, messageId]);
  }

  observe(token: VisibleMaterialToken, settled: boolean): void {
    const changed = settled ? this.#presenting.delete(token) : !this.#presenting.has(token);
    if (!settled && changed) this.#presenting.add(token);
    if (changed) this.#publish();
  }

  retire(token: VisibleMaterialToken): void {
    if (!this.#presenting.delete(token)) return;
    this.#publish();
  }

  actionsMaterialization(source: MessageActionMaterialization): MessageActionMaterialization {
    return source === "active" || this.#presenting.size > 0 ? "active" : "settled";
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

const MessageVisibleMaterialContext = createContext<MessageVisibleMaterialOwner | null>(null);

export function MessageVisibleMaterialProvider({
  owner,
  children,
}: {
  owner: MessageVisibleMaterialOwner;
  children: ReactNode;
}) {
  return (
    <MessageVisibleMaterialContext.Provider value={owner}>
      {children}
    </MessageVisibleMaterialContext.Provider>
  );
}

/** Re-render message chrome only when a visible projection crosses the
 * presenting/settled boundary; token-by-token text growth stays local. */
export function useVisibleActionMaterialization(
  owner: MessageVisibleMaterialOwner,
  source: MessageActionMaterialization,
): MessageActionMaterialization {
  useSyncExternalStore(owner.subscribe, owner.snapshot, owner.snapshot);
  return owner.actionsMaterialization(source);
}

/** Bind one mounted visible-text projection to the exact message owner. */
export function useVisibleTextMaterial(settled: boolean): void {
  const owner = useContext(MessageVisibleMaterialContext);
  const [token] = useState<VisibleMaterialToken>(() => Symbol("visible-text-material"));

  useLayoutEffect(() => {
    if (!owner) return;
    owner.observe(token, settled);
    return () => owner.retire(token);
  }, [owner, settled, token]);
}
