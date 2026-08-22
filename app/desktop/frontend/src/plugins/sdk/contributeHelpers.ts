// Layout contributions need a stable id so re-registering a slot entry replaces
// rather than stacks.
// Free functions over an explicit ctx, not methods on an ambient host — the
// difference being that a plugin has to import what it uses.

import type { Contributor } from "./definePlugin";
import { LAYOUT_SLOT } from "./kernelPoints";
import type { Disposable } from "./types/common";
import type { LayoutSlotSpec } from "./types/workspace";

export function contributeLayout(ctx: Contributor, slot: string, spec: LayoutSlotSpec): Disposable {
  return ctx.contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` });
}
