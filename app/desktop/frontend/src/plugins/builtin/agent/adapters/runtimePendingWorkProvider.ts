import { getContainer } from "@/main/container";
import type { ContributingHost } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { PENDING_WORK_KEY, pendingWorkItems } from "../application/hitl/pendingWork";
import { runtimePendingInterruptSet } from "./runtimeAgentFacts";

/** The install-wide HITL queue belongs to the Agent context. Runtime paging and
 * wire translation stop here before the pending-work read model is derived. */
export function contributeRuntimePendingWork(host: ContributingHost): void {
  host.extensions.contribute(DATA_PROVIDER, {
    key: PENDING_WORK_KEY,
    fetcher: async () =>
      pendingWorkItems(
        (await getContainer().client().interrupts.list().autoPagingToArray()).map(
          runtimePendingInterruptSet,
        ),
      ),
  });
}
