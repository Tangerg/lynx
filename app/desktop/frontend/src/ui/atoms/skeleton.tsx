// Skeleton primitives for loading states. Only `SkeletonList` is exported;
// `Line` + `Row` are internal building blocks.
//
// The shimmer is a translated overlay, not a scrolling background. `background-position`
// repaints the whole element every frame, and a list is eight of these at once — on the
// main thread, usually beside a streaming transcript. A transform costs nothing per
// frame because the compositor owns it. Honors prefers-reduced-motion via
// `motion-reduce:animate-none` on the moving part.

import type { CSSProperties } from "react";

function SkeletonLine({ width = "100%", height = 10 }: { width?: string; height?: number }) {
  return (
    <span
      className="relative inline-block overflow-hidden rounded-xs bg-surface-2"
      style={{ width, height }}
    >
      <span
        aria-hidden
        className={
          "absolute inset-0 animate-sweep motion-reduce:animate-none " +
          "bg-[linear-gradient(100deg,transparent_0%,var(--color-surface)_50%,transparent_100%)]"
        }
      />
    </span>
  );
}

function SkeletonRow({ variant }: { variant: SkeletonListVariant }) {
  if (variant === "compact") {
    return (
      <div className="flex h-7 items-center gap-2 px-2">
        <SkeletonLine width="14px" height={14} />
        <SkeletonLine width="62%" height={8} />
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1.5 py-2">
      <SkeletonLine width="68%" />
      <SkeletonLine width="38%" height={8} />
    </div>
  );
}

export type SkeletonListVariant = "stacked" | "compact";

export function SkeletonList({
  count = 4,
  style,
  label = "Loading…",
  variant = "stacked",
}: {
  count?: number;
  style?: CSSProperties;
  /** Screen-reader announcement. Default matches the visual shimmer's intent. */
  label?: string;
  /** Compact rows fit navigation surfaces; stacked rows fit content lists. */
  variant?: SkeletonListVariant;
}) {
  return (
    <output
      className={
        variant === "compact" ? "flex flex-col gap-0.5 py-1" : "flex flex-col gap-2 px-3 py-2"
      }
      style={style}
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">{label}</span>
      {Array.from({ length: count }, (_, i) => (
        <SkeletonRow key={i} variant={variant} />
      ))}
    </output>
  );
}
