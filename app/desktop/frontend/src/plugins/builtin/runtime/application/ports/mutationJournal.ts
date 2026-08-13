import { createSingletonPort } from "@/lib/ports/singletonPort";

/** Runtime context persistence capability. The Application port owns only an
 * opaque snapshot boundary; RPC decides the snapshot schema and the Adapter
 * decides where it is stored. */
export interface RuntimeMutationSnapshotStorage {
  read(): unknown;
  write(snapshot: unknown): void;
}

const port = createSingletonPort<RuntimeMutationSnapshotStorage>(
  "Runtime mutation journal storage is not installed",
);

export const configureRuntimeMutationJournalStorage = port.configure;
export const installedRuntimeMutationJournalStorage = port.peek;
