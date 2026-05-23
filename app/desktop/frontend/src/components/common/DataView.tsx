// DataView — the canonical "loading skeleton | empty state | content"
// tri-state branch every query-driven surface in the app was open-coding.
//
// Replaces a repeated ~10-line ternary chain:
//
//   {isLoading
//     ? <SkeletonList count={6} />
//     : !data || data.length === 0
//       ? <EmptyState icon="..." title="..." sub="..." />
//       : <WhateverContent items={data} />
//   }
//
// with a render-prop component that owns the branching once:
//
//   <DataView items={data} isLoading={isLoading} skeletonCount={6}
//     empty={{ icon: "...", title: "...", sub: "..." }}>
//     {(items) => <WhateverContent items={items} />}
//   </DataView>
//
// The body wrapper (div.side-list, ScrollArea, FilesChanged, …) is the
// consumer's call — DataView doesn't presume how the success state
// should render. That keeps it useful for the wildly different layouts
// each surface wants (sidebar row list, scrollable tree, terminal lines).

import type { ReactNode } from "react";
import { EmptyState } from "./EmptyState";
import type { IconName } from "./Icon";
import { SkeletonList } from "./Skeleton";

type EmptyConfig = {
  icon?: IconName;
  title: string;
  sub?: string;
  size?: "compact" | "comfortable";
};

type Props<T> = {
  /** Query result. `undefined` is treated the same as `null` / empty list. */
  items: T[] | undefined;
  /** True while the underlying query is loading for the first time. */
  isLoading: boolean;
  /** Number of skeleton rows to render while loading. Defaults to 4. */
  skeletonCount?: number;
  /**
   * Empty-state config. Omit to render nothing on empty (rare — most
   * surfaces benefit from an explicit "nothing here yet" message).
   */
  empty?: EmptyConfig;
  /** Renderer for the success branch — receives the non-empty items list. */
  children: (items: T[]) => ReactNode;
};

export function DataView<T>({
  items,
  isLoading,
  skeletonCount = 4,
  empty,
  children,
}: Props<T>) {
  if (isLoading) return <SkeletonList count={skeletonCount} />;
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} /> : null;
  }
  return <>{children(items)}</>;
}
