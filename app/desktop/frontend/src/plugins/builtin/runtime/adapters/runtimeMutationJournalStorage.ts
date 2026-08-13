import type { Host } from "@/plugins/sdk";
import { configureRuntimeMutationJournalStorage } from "../application/ports/mutationJournal";

const STORAGE_KEY = "mutation-journal-v1";

/** Bind the RPC mutation journal to this Runtime context's namespaced Host
 * storage. The adapter never interprets protocol methods, params, or keys. */
export function installRuntimeMutationJournalStorage(host: Pick<Host, "storage">): () => void {
  return configureRuntimeMutationJournalStorage({
    read: () => host.storage.get(STORAGE_KEY),
    write: (snapshot) => host.storage.set(STORAGE_KEY, snapshot),
  });
}
