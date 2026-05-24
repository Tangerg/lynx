// Skeleton primitives for loading states. Three shapes cover the common
// cases:
//   <SkeletonLine width={"60%"} /> — a single inline row
//   <SkeletonRow />                — full-width line + secondary line
//   <SkeletonList count={5} />     — N stacked rows
//
// Shimmer animation uses the `animate-shimmer` keyframe declared in
// styles/globals.css. Honors prefers-reduced-motion via Tailwind's
// `motion-reduce:animate-none` modifier.

import type { CSSProperties } from "react";

type LineProps = {
  width?: string;
  height?: number;
  style?: CSSProperties;
};

export function SkeletonLine({ width = "100%", height = 10, style }: LineProps) {
  return (
    <span
      className={
        "inline-block rounded animate-shimmer motion-reduce:animate-none " +
        "bg-[linear-gradient(90deg,var(--color-surface-2)_0%,color-mix(in_srgb,var(--color-text)_8%,var(--color-surface-2))_50%,var(--color-surface-2)_100%)] " +
        "bg-[length:200%_100%]"
      }
      style={{ width, height, ...style }}
    />
  );
}

export function SkeletonRow({ style }: { style?: CSSProperties }) {
  return (
    <div className="flex flex-col gap-1.5 py-2" style={style}>
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
