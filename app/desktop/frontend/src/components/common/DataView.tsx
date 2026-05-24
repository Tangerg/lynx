// Render-prop wrapper for the "loading skeleton | empty state | content"
// tri-state branch every query-driven surface needs. Owns the branching
// once; the success-state render is the consumer's call (sidebar row,
// scrollable tree, terminal lines — different layouts per surface).

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

export function DataView<T>({ items, isLoading, skeletonCount = 4, empty, children }: Props<T>) {
  if (isLoading) return <SkeletonList count={skeletonCount} />;
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} /> : null;
  }
  return <>{children(items)}</>;
}
