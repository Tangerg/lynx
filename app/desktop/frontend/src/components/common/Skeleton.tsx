// Skeleton primitives for loading states. Three shapes cover the common
// cases:
//   <SkeletonLine width={"60%"} /> — a single inline row
//   <SkeletonRow />                — full-width line + secondary line
//   <SkeletonList count={5} />     — N stacked rows
//
// Animation is a shimmer (linear-gradient sweep) defined in style.css —
// `keyframes lyra-shimmer`. CSS lives there so the animation can be
// reduced via `prefers-reduced-motion` without touching JS.

import type { CSSProperties } from "react";

type LineProps = {
  width?: string;
  height?: number;
  style?: CSSProperties;
};

export function SkeletonLine({ width = "100%", height = 10, style }: LineProps) {
  return (
    <span
      className="skeleton-line"
      style={{ width, height, ...style }}
    />
  );
}

export function SkeletonRow({ style }: { style?: CSSProperties }) {
  return (
    <div className="skeleton-row" style={style}>
      <SkeletonLine width="68%" />
      <SkeletonLine width="38%" height={8} />
    </div>
  );
}

export function SkeletonList({ count = 4, style }: { count?: number; style?: CSSProperties }) {
  return (
    <div className="skeleton-list" style={style}>
      {Array.from({ length: count }, (_, i) => <SkeletonRow key={i} />)}
    </div>
  );
}
