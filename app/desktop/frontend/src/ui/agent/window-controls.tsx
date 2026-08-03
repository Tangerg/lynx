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
  /** Which mark the zoom control wears: arrows out to fill the screen, arrows in
   *  to come back. A control showing the same one in both states is describing
   *  what it does half the time. */
  maximised: boolean;
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
 * the hues, the glyph metrics, the 50% black ink, revealing all three glyphs when
 * the pointer enters the cluster rather than one at a time, and greying out while
 * the window is not the one you are working in. These marks are the most
 * recognised control on the desktop; anything "nicer" here reads as a knock-off,
 * not as taste — so the values are sampled off a real title bar rather than
 * remembered, and the numbers everyone quotes for them turned out to be wrong.
 */
export function AgentWindowControls({
  onClose,
  onMinimise,
  onToggleMaximise,
  closeLabel,
  minimiseLabel,
  maximiseLabel,
  maximised,
}: AgentWindowControlsProps) {
  return (
    <div className="group/window flex items-center" data-slot="agent-window-controls">
      {/* Every number below is measured off a real title bar, not judged by eye:
          the cross spans half the disc on a 1.75px stroke, the bar 57% on the
          same, and the zoom mark 43%. Guessing these produced a cross a third
          too small on a stroke half too thin, which is the whole difference
          between a window control and a drawing of one. */}
      <WindowControl label={closeLabel} onClick={onClose} hue="close">
        <path d="M3.75 3.75 L8.25 8.25 M8.25 3.75 L3.75 8.25" />
      </WindowControl>
      <WindowControl label={minimiseLabel} onClick={onMinimise} hue="minimise">
        <path d="M3.25 6 H8.75" />
      </WindowControl>
      <WindowControl label={maximiseLabel} onClick={onToggleMaximise} hue="maximise">
        {/* Two triangles either side of a bottom-left-to-top-right split. Pointing
            out at their own corners they read as "fill the screen"; mirrored to
            point at each other they read as "come back", and the platform draws
            that second pair noticeably larger — 68% of the disc against 43%. */}
        {maximised ? (
          <>
            <path d="M5.8 2 V5.8 H2 Z" fill="currentColor" stroke="none" />
            <path d="M6.2 10 V6.2 H10 Z" fill="currentColor" stroke="none" />
          </>
        ) : (
          <>
            <path d="M3.45 3.45 H7.9 L3.45 7.9 Z" fill="currentColor" stroke="none" />
            <path d="M8.55 8.55 H4.1 L8.55 4.1 Z" fill="currentColor" stroke="none" />
          </>
        )}
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
      // 14px mark on a 20px pitch, target butted edge to edge — the platform's
      // geometry to the pixel, measured off a real title bar beside ours at the
      // same scale rather than taken from the 12pt everyone quotes. It is under WCAG 2.2's 24px minimum, and that is
      // the one place here we take the exception rather than the guideline: this
      // cluster is the most recognised control on the desktop, its size and
      // spacing ARE the recognition, and every macOS user hits it all day. The
      // audit excludes it by name and says so.
      //
      // No `title`: the platform's controls carry no tooltip, and one popping up
      // over the drawer every time the pointer crosses the corner is the loudest
      // possible tell. The accessible name is on `aria-label`.
      className="grid size-5 place-items-center rounded-full"
    >
      <span
        // A flat disc, no rim. The darker ring everyone draws under these is not
        // there on the real thing — sampling one shows the edge pixel is the fill
        // colour, and what reads as a ring is the antialiased edge against the bar.
        className={cn(
          "grid size-3.5 place-items-center rounded-full",
          "bg-[var(--fill)] transition-colors duration-[var(--dur-fast)]",
          "[:root[data-window-inactive]_&]:bg-[var(--window-control-inactive)]",
        )}
        style={{ "--fill": `var(--window-control-${hue})` } as CSSProperties}
      >
        <svg
          viewBox="0 0 12 12"
          aria-hidden
          className={cn(
            "size-3.5 text-[var(--window-control-ink)] opacity-0 transition-opacity duration-[var(--dur-fast)]",
            "group-hover/window:opacity-100",
            "[:root[data-window-inactive]_&]:opacity-0",
          )}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.75"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {children}
        </svg>
      </span>
    </Pressable>
  );
}
