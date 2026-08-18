import type { CSSProperties } from "react";
import { useCallback, useLayoutEffect, useRef, useState } from "react";

interface OverflowLabelStyle extends CSSProperties {
  "--agent-overflow-distance": string;
  "--agent-overflow-duration": string;
}

interface Props {
  text: string;
}

/**
 * A single-line label that reveals overflow the same way the Codex task row
 * does: measure the real rendered text, then move only the inner track on the
 * owning row's hover/focus. The title itself stays one DOM/text identity; the
 * caller may use that same string for the row's accessible name and hover card.
 */
export function AgentOverflowLabel({ text }: Props) {
  const viewportRef = useRef<HTMLSpanElement>(null);
  const trackRef = useRef<HTMLSpanElement>(null);
  const [geometry, setGeometry] = useState({ distance: 0, duration: 0 });

  const measure = useCallback(() => {
    const viewport = viewportRef.current;
    const track = trackRef.current;
    if (!viewport || !track) return;

    const distance = Math.max(0, track.getBoundingClientRect().width - viewport.clientWidth);
    const fontSize = Number.parseFloat(getComputedStyle(track).fontSize) || 13;
    // Codex's task marquee advances at two em per second and stops at the end.
    const duration = distance > 0 ? distance / (fontSize * 2) : 0;
    setGeometry((current) =>
      Math.abs(current.distance - distance) < 0.5 && Math.abs(current.duration - duration) < 0.01
        ? current
        : { distance, duration },
    );
  }, []);

  useLayoutEffect(() => {
    measure();
    if (typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver(measure);
    if (viewportRef.current) observer.observe(viewportRef.current);
    if (trackRef.current) observer.observe(trackRef.current);
    return () => observer.disconnect();
  }, [measure, text]);

  const style: OverflowLabelStyle = {
    "--agent-overflow-distance": `${geometry.distance}px`,
    "--agent-overflow-duration": `${geometry.duration}s`,
  };

  return (
    <span
      ref={viewportRef}
      className="agent-overflow-label min-w-0 flex-1 truncate-fade"
      data-overflowing={geometry.distance > 0 ? "" : undefined}
    >
      <span
        ref={trackRef}
        className="agent-overflow-track inline-block min-w-max"
        data-overflowing={geometry.distance > 0 ? "" : undefined}
        style={style}
      >
        {text}
      </span>
    </span>
  );
}
