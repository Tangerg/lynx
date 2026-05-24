// Skeleton primitives for loading states. Only `SkeletonList` is exported;
// `Line` + `Row` are internal building blocks. Shimmer uses the
// `animate-shimmer` keyframe in styles/globals.css and honors
// prefers-reduced-motion via `motion-reduce:animate-none`.

import type { CSSProperties } from "react";

function SkeletonLine({
  width = "100%",
  height = 10,
}: {
  width?: string;
  height?: number;
}) {
  return (
    <span
      className={
        "inline-block rounded animate-shimmer motion-reduce:animate-none " +
        "bg-[linear-gradient(90deg,var(--color-surface-2)_0%,color-mix(in_srgb,var(--color-text)_8%,var(--color-surface-2))_50%,var(--color-surface-2)_100%)] " +
        "bg-[length:200%_100%]"
      }
      style={{ width, height }}
    />
  );
}

function SkeletonRow() {
  return (
    <div className="flex flex-col gap-1.5 py-2">
      <SkeletonLine width="68%" />
      <SkeletonLine width="38%" height={8} />
    </div>
  );
}

export function SkeletonList({ count = 4, style }: { count?: number; style?: CSSProperties }) {
  return (
    <div className="flex flex-col gap-2 px-3 py-2" style={style}>
      {Array.from({ length: count }, (_, i) => (
        <SkeletonRow key={i} />
      ))}
    </div>
  );
}
