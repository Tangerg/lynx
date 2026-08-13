import { createSingletonPort } from "@/lib/ports/singletonPort";

/** Runtime context persistence capability. The Application port owns only an
 * opaque key/value boundary; RPC decides the records and the Adapter decides
 * where they are stored. */
export interface RuntimeMutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}

const port = createSingletonPort<RuntimeMutationJournalStorage>(
  "Runtime mutation journal storage is not installed",
);

export const configureRuntimeMutationJournalStorage = port.configure;
export const installedRuntimeMutationJournalStorage = port.peek;
