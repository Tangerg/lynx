import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Pressable } from "@/ui/atoms/pressable";

interface AgentWindowControlsProps {
  onClose: () => void;
  onMinimise: () => void;
  onToggleMaximise: () => void;
  closeLabel: string;
  minimiseLabel: string;
  maximiseLabel: string;
}

/**
 * The window's three controls, drawn by the app.
 *
 * The platform draws none: this window has no title bar, because a title bar's
 * buttons sit in a box only the system knows the size of — it moves with the
 * appearance and with whether a toolbar exists, and nothing inside the window
 * can measure it. Every attempt to align our chrome against that box was a guess
 * that held until it didn't. Owning the three marks makes the gutter arithmetic
 * ours.
 *
 * Everything else about them is the platform's, deliberately and to the value:
 * the hues, the darker rim on each disc, the glyph set, revealing all three
 * glyphs when the pointer enters the cluster rather than one at a time, and
 * greying out while the window is not the one you are working in. These marks
 * are the most recognised control on the desktop; anything "nicer" here reads as
 * a knock-off, not as taste.
 */
export function AgentWindowControls({
  onClose,
  onMinimise,
  onToggleMaximise,
  closeLabel,
  minimiseLabel,
  maximiseLabel,
}: AgentWindowControlsProps) {
  return (
    <div className="group/window flex items-center" data-slot="agent-window-controls">
      <WindowControl label={closeLabel} onClick={onClose} hue="close">
        <path d="M4.15 4.15 L7.85 7.85 M7.85 4.15 L4.15 7.85" strokeWidth="1.5" />
      </WindowControl>
      <WindowControl label={minimiseLabel} onClick={onMinimise} hue="minimise">
        <path d="M3.45 6 H8.55" strokeWidth="1.5" />
      </WindowControl>
      <WindowControl label={maximiseLabel} onClick={onToggleMaximise} hue="maximise">
        {/* Two triangles either side of a bottom-left-to-top-right split — the
            platform's zoom mark. A plain square outline is a different symbol.
            Stroked as well as filled, at a hair's width, so the corners round the
            way every other glyph here does. */}
        <path d="M4.1 4.1 H7.5 L4.1 7.5 Z" fill="currentColor" strokeWidth="0.55" />
        <path d="M7.9 7.9 H4.5 L7.9 4.5 Z" fill="currentColor" strokeWidth="0.55" />
      </WindowControl>
    </div>
  );
}

function WindowControl({
  label,
  onClick,
  hue,
  children,
}: {
  label: string;
  onClick: () => void;
  hue: "close" | "minimise" | "maximise";
  children: ReactNode;
}) {
  return (
    <Pressable
      type="button"
      aria-label={label}
      onClick={onClick}
      // 24px target around a 12px mark, butted edge to edge so the pitch IS the
      // target. The platform draws 12px marks 20px apart and exempts itself from
      // the 24px minimum; we do not get that exemption, and the four extra pixels
      // of pitch buy a control you can actually hit without aiming.
      //
      // No `title`: the platform's controls carry no tooltip, and one popping up
      // over the drawer every time the pointer crosses the corner is the loudest
      // possible tell. The accessible name is on `aria-label`.
      className="grid size-6 place-items-center rounded-full"
    >
      <span
        className={cn(
          "grid size-3 place-items-center rounded-full border-[0.5px]",
          "border-[color:var(--edge)] bg-[var(--fill)] transition-colors duration-[var(--dur-fast)]",
          "[:root[data-window-inactive]_&]:border-transparent",
          "[:root[data-window-inactive]_&]:bg-fg-faint/30",
        )}
        style={
          {
            "--fill": `var(--window-control-${hue})`,
            "--edge": `var(--window-control-${hue}-edge)`,
          } as CSSProperties
        }
      >
        <svg
          viewBox="0 0 12 12"
          aria-hidden
          className={cn(
            "size-3 text-black/55 opacity-0 transition-opacity duration-[var(--dur-fast)]",
            "group-hover/window:opacity-100",
            "[:root[data-window-inactive]_&]:opacity-0",
          )}
          fill="none"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {children}
        </svg>
      </span>
    </Pressable>
  );
}
