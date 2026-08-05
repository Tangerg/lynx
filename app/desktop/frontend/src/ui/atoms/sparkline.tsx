import { useId } from "react";
import { cn } from "@/lib/classNames";

interface SparklineProps {
  /** At least two points; fewer is not a shape and renders nothing. */
  data: readonly number[];
  /** What the line is, for anyone who cannot see it. A sparkline with no name is a
   *  decoration, and a decoration should not be in the accessibility tree at all. */
  label: string;
  className?: string;
}

/**
 * A trend, at the size of a word.
 *
 * `currentColor` and no fill by default: the line belongs to whatever ink its row is
 * set in, so it follows the tone the row already chose rather than introducing a
 * colour of its own. That is the whole reason this is a component and not a chart
 * library — a chart brings a palette, and a palette here would be a second answer to
 * "what colour is this state".
 */
export function Sparkline({ data, label, className }: SparklineProps) {
  const gradientId = useId();
  if (data.length < 2) return null;

  const min = Math.min(...data);
  const max = Math.max(...data);
  // A flat series has no range to divide by, and drawing it along the bottom edge
  // would read as zero rather than as unchanged — so it runs down the middle.
  const range = max - min || 1;
  const flat = max === min;

  const points = data.map((value, index) => {
    const x = (index / (data.length - 1)) * 100;
    const y = flat ? 50 : 100 - ((value - min) / range) * 100;
    return `${x},${y}`;
  });

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      className={cn("h-4 w-12 overflow-visible", className)}
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.18" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={`0,100 ${points.join(" ")} 100,100`} fill={`url(#${gradientId})`} />
      <polyline
        points={points.join(" ")}
        fill="none"
        stroke="currentColor"
        strokeWidth="6"
        // Non-scaling stroke, because the viewBox is stretched to the element's box:
        // without it the line is thick horizontally and hairline vertically.
        vectorEffect="non-scaling-stroke"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
