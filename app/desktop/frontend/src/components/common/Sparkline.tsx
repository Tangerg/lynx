// Sparkline — tiny SVG line chart for status bar / tool card data
// readouts. Built for "I'm a number that has history" places: token
// usage per step, cost-per-step, tool duration trend, etc.
//
// Direction 2 (Bloomberg data density): the value is the ratio of points
// to chrome — keep this primitive ruthlessly minimal. No axes, no
// labels, no animation. Just the line.

import type { CSSProperties } from "react";

interface Props {
  values: number[];
  width?: number;
  height?: number;
  /** Stroke colour. Defaults to current text colour. */
  color?: string;
  /** Fill the area below the line at low opacity. */
  fill?: boolean;
  /** Extra CSS — useful for vertical-align nudges next to text. */
  style?: CSSProperties;
}

export function Sparkline({ values, width = 48, height = 14, color, fill = false, style }: Props) {
  if (values.length < 2) {
    // One or zero data points → render a flat baseline at mid-height so
    // the slot doesn't visually collapse.
    return (
      <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} style={style}>
        <line
          x1={0}
          y1={height / 2}
          x2={width}
          y2={height / 2}
          stroke={color ?? "currentColor"}
          strokeWidth={1}
          strokeOpacity={0.35}
        />
      </svg>
    );
  }

  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  // 1px inset so the stroke doesn't get clipped at the edges.
  const w = width - 2;
  const h = height - 2;
  const stepX = w / (values.length - 1);
  const points = values
    .map((v, i) => {
      const x = 1 + i * stepX;
      const y = 1 + h - ((v - min) / range) * h;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(" ");

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} style={style}>
      {fill && (
        <polygon
          points={`1,${height - 1} ${points} ${width - 1},${height - 1}`}
          fill={color ?? "currentColor"}
          fillOpacity={0.12}
        />
      )}
      <polyline
        points={points}
        fill="none"
        stroke={color ?? "currentColor"}
        strokeWidth={1.25}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}
