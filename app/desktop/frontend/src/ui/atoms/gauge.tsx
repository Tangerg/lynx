import { cn } from "@/lib/classNames";

const CIRCUMFERENCE = 2 * Math.PI * 6;

/**
 * A donut reading one bounded ratio.
 *
 * Deliberately silent: no number, no unit, no label of its own. It exists for the
 * places a figure has to be readable at a glance from a control row without
 * reflowing it — the fill is the whole message, and whatever the number is goes in
 * the caller's tooltip. A gauge that printed its own value would grow and shrink
 * with the digits, which is precisely what a fixed glyph is for.
 *
 * Sized off `--icon-sm`, so it sits on the icon ladder and picks up whatever
 * metrics its region declares rather than carrying a pixel of its own.
 */
export function Gauge({
  value,
  label,
  className,
}: {
  /** 0…1. Values outside are clamped — a ratio that overshoots is still "full". */
  value: number;
  /** The accessible name. This is an image, so it needs one. */
  label: string;
  className?: string;
}) {
  const ratio = Math.min(1, Math.max(0, value));
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox="0 0 16 16"
      // The dial starts at twelve and runs clockwise — the only reading nobody has
      // to learn; SVG starts an arc at three. Rotating the whole element rather
      // than the arc is exact here (the track is a full circle, so it is
      // rotationally symmetric) and keeps the transform on the class ladder
      // instead of in a presentation attribute.
      className={cn("size-[var(--icon-sm)] shrink-0 -rotate-90", className)}
    >
      <circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        // The dial and the arc are one mark, so the track follows the caller's
        // colour (see `--gauge-track`). Hung on `--color-border` it vanished:
        // that token is a hairline for chrome, and this is data.
        stroke="var(--gauge-track)"
        strokeWidth="2.5"
        vectorEffect="non-scaling-stroke"
      />
      <circle
        cx="8"
        cy="8"
        r="6"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeDasharray={`${ratio * CIRCUMFERENCE} ${CIRCUMFERENCE}`}
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
