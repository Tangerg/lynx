// Thin Radix Tooltip wrapper with project-wide visual defaults. Use
// it via the named import:
//
//   <Tooltip label="Toggle sidebar (⌘B)" side="bottom">
//     <button>…</button>
//   </Tooltip>
//
// Or via the convenience attribute pattern: any component that accepts
// a `title` prop and embeds <button> internally (IconButton, Chip) wires
// up a tooltip automatically when `title` is set. Native HTML `title`
// is no longer used anywhere visible in the kernel — Radix renders a
// portal'd panel after ~250ms hover with keyboard focus support, vs
// the OS-default 1–2s lag + zero focus support.
//
// Tooltip.Provider is mounted once at the app root (PluginProvider),
// so callers never need to wrap their own provider.

import type { ReactNode } from "react";
import * as RadixTooltip from "@radix-ui/react-tooltip";

interface Props {
  /** Tooltip label. When empty / undefined, renders children unwrapped
   *  so call sites can conditionally enable the tooltip without branching. */
  label?: ReactNode;
  /** Preferred side relative to the trigger. Radix flips automatically
   *  near a viewport edge. */
  side?: "top" | "right" | "bottom" | "left";
  /** Distance in pixels between the trigger and the tooltip card. */
  sideOffset?: number;
  /** Override the hover-open delay. Default 250ms — snappier than the
   *  Radix default 700ms (matches the native `title` lag we replaced). */
  delayDuration?: number;
  children: ReactNode;
}

export function Tooltip({ label, side = "top", sideOffset = 6, delayDuration, children }: Props) {
  if (label == null || label === "") return <>{children}</>;
  // Nested provider is fine — Radix scopes Trigger ↔ Root and the
  // outer provider (mounted in PluginProvider) just supplies a default
  // delay. Including one here keeps the wrapper safe to use in tests +
  // standalone Storybook contexts without dragging in app-level setup.
  return (
    <RadixTooltip.Provider delayDuration={delayDuration ?? 250}>
      <RadixTooltip.Root>
        <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
        <RadixTooltip.Portal>
          <RadixTooltip.Content
            side={side}
            sideOffset={sideOffset}
            // Keep the panel narrow so long labels wrap into 2-3 lines
            // instead of stretching across the viewport.
            className="z-50 max-w-[280px] rounded-md border border-line-soft bg-surface px-2 py-1 font-sans text-[11.5px] leading-snug text-fg-soft shadow-[var(--shadow-elevated)] animate-rise-in"
          >
            {label}
          </RadixTooltip.Content>
        </RadixTooltip.Portal>
      </RadixTooltip.Root>
    </RadixTooltip.Provider>
  );
}
