import type { ReactNode } from "react";
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
 * The geometry stays the platform's, because that is what a hand already knows:
 * 12px marks on a 20px pitch, glyphs revealed by hovering the group rather than
 * the single mark, and the cluster going grey while the window is not the one
 * you are working in.
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
      <WindowControl label={closeLabel} onClick={onClose} tint="var(--window-control-close)">
        <path d="M4.2 4.2 L7.8 7.8 M7.8 4.2 L4.2 7.8" />
      </WindowControl>
      <WindowControl
        label={minimiseLabel}
        onClick={onMinimise}
        tint="var(--window-control-minimise)"
      >
        <path d="M3.6 6 H8.4" />
      </WindowControl>
      <WindowControl
        label={maximiseLabel}
        onClick={onToggleMaximise}
        tint="var(--window-control-maximise)"
      >
        <path d="M4.3 4.3 H7.7 V7.7 H4.3 Z" />
      </WindowControl>
    </div>
  );
}

function WindowControl({
  label,
  onClick,
  tint,
  children,
}: {
  label: string;
  onClick: () => void;
  tint: string;
  children: ReactNode;
}) {
  return (
    <Pressable
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      // 24px target around a 12px mark, butted edge to edge so the pitch IS the
      // target. The platform draws 12px marks 20px apart and exempts itself from
      // the 24px minimum; we do not get that exemption, and the four extra pixels
      // of pitch buy a control you can actually hit without aiming.
      className="grid size-6 place-items-center rounded-full"
    >
      <span
        className={cn(
          "grid size-3 place-items-center rounded-full",
          "bg-[var(--tint)] transition-colors duration-[var(--dur-fast)]",
          "[:root[data-window-inactive]_&]:bg-fg-faint/30",
        )}
        style={{ "--tint": tint } as React.CSSProperties}
      >
        <svg
          viewBox="0 0 12 12"
          aria-hidden
          className={cn(
            "size-3 opacity-0 transition-opacity duration-[var(--dur-fast)]",
            "stroke-black/55 group-hover/window:opacity-100",
            "[:root[data-window-inactive]_&]:opacity-0",
          )}
          fill="none"
          strokeWidth="1.3"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {children}
        </svg>
      </span>
    </Pressable>
  );
}
