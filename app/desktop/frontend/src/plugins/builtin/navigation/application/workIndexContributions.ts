import type {
  WorkIndexItemPlacement,
  WorkIndexItemScope,
  WorkIndexItemSpec,
} from "@/plugins/sdk/types/navigation";
import type { Disposable } from "@/plugins/sdk/types/common";
import type { Host } from "@/plugins/sdk/types/host";
import { useWorkIndexItems as useSdkWorkIndexItems } from "@/plugins/sdk/selectors/layout";
import { WORK_INDEX_ITEM } from "@/plugins/sdk/kernelPoints";

export type {
  WorkIndexItemPlacement,
  WorkIndexItemScope,
  WorkIndexItemSpec,
} from "@/plugins/sdk/types/navigation";

export function contributeWorkIndexItem(host: Host, item: WorkIndexItemSpec): Disposable {
  return host.extensions.contribute(WORK_INDEX_ITEM, item);
}

export function useWorkIndexItems(
  placement: WorkIndexItemPlacement,
  scope?: WorkIndexItemScope,
): WorkIndexItemSpec[] {
  return useSdkWorkIndexItems(placement, scope);
}
