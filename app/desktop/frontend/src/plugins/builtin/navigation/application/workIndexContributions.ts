import type {
  WorkIndexItemScope,
  WorkIndexItemSpec,
  WorkIndexItemVariant,
} from "@/plugins/sdk/types/navigation";
import type { Disposable } from "@/plugins/sdk/types/common";
import type { Host } from "@/plugins/sdk/types/host";
import { useWorkIndexItems as useSdkWorkIndexItems } from "@/plugins/sdk/selectors/layout";
import { WORK_INDEX_ITEM } from "@/plugins/sdk/kernelPoints";

export type {
  WorkIndexItemScope,
  WorkIndexItemSpec,
  WorkIndexItemVariant,
} from "@/plugins/sdk/types/navigation";

export function contributeWorkIndexItem(host: Host, item: WorkIndexItemSpec): Disposable {
  return host.extensions.contribute(WORK_INDEX_ITEM, item);
}

export function useWorkIndexItems(
  variant: WorkIndexItemVariant,
  scope?: WorkIndexItemScope,
): WorkIndexItemSpec[] {
  return useSdkWorkIndexItems(variant, scope);
}
