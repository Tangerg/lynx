// Render-prop wrapper for the "loading skeleton | empty state | content"
// tri-state branch every query-driven surface needs. Owns the branching
// once; the success-state render is the consumer's call (sidebar row,
// scrollable tree, terminal lines — different layouts per surface).

import type { ReactNode } from "react";
import type { IconName } from "./Icon";
import { EmptyState } from "./EmptyState";
import { SkeletonList } from "./Skeleton";

interface EmptyConfig {
  icon?: IconName;
  title: string;
  sub?: string;
  size?: "compact" | "comfortable";
}

interface Props<T> {
  /** Query result. `undefined` is treated the same as `null` / empty list. */
  items: T[] | undefined;
  /** True while the underlying query is loading for the first time. */
  isLoading: boolean;
  /**
   * True when the underlying query rejected. Without this, a failed fetch
   * (backend down, 401, capability_not_negotiated, no data provider) falls
   * through to the empty branch and masks a hard error as "nothing here yet".
   */
  isError?: boolean;
  /** Number of skeleton rows to render while loading. Defaults to 4. */
  skeletonCount?: number;
  /**
   * Empty-state config. Omit to render nothing on empty (rare — most
   * surfaces benefit from an explicit "nothing here yet" message).
   */
  empty?: EmptyConfig;
  /**
   * Override the error-state copy. Defaults to a generic "couldn't load"
   * message — pass this only when a surface needs domain-specific wording.
   */
  error?: EmptyConfig;
  /** Renderer for the success branch — receives the non-empty items list. */
  children: (items: T[]) => ReactNode;
}

export function DataView<T>({
  items,
  isLoading,
  isError,
  skeletonCount = 4,
  empty,
  error,
  children,
}: Props<T>) {
  if (isLoading) return <SkeletonList count={skeletonCount} />;
  if (isError) {
    return (
      <EmptyState
        icon="alert"
        title="Couldn’t load"
        sub="The runtime didn’t answer this request. Check the connection and retry."
        {...error}
      />
    );
  }
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} /> : null;
  }
  return <>{children(items)}</>;
}
