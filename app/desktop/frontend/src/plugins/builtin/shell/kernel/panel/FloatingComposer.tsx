import type { ReactNode, RefObject } from "react";
import { AnimatePresence, motion } from "motion/react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { disclosureTransition } from "@/lib/motion";
import { useRuntimeServiceStatus } from "@/plugins/builtin/runtime/public/serviceStatus";
import { CONNECTION_PANE } from "@/plugins/builtin/settings/public/panes";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { Slot } from "@/plugins/host/Slot";
import { SystemMessage } from "@/ui";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { READING_COLUMN, READING_GUTTER } from "./readingColumn";

/** Connection feedback belongs beside the controls whose rights it explains.
 * Cold-start checking stays silent; only a proven loss or terminal failed attempt
 * produces material. The connection owner supplies that distinction directly. */
export function RuntimeConnectionNotice() {
  const t = useT();
  const service = useRuntimeServiceStatus();
  const visible = service.phase === "reconnecting" || service.phase === "unavailable";
  const unavailable = service.phase === "unavailable";

  return (
    <AnimatePresence initial={false}>
      {visible && (
        <motion.div
          key="runtime-connection-notice"
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 2 }}
          transition={disclosureTransition}
          className="mb-2"
        >
          <SystemMessage
            variant={unavailable ? "error" : "warning"}
            icon={unavailable ? "alert" : "loop"}
            role={unavailable ? "alert" : "status"}
            aria-live={unavailable ? "assertive" : "polite"}
            className="text-pretty"
            action={
              unavailable
                ? {
                    label: t("runtime.connection.settings"),
                    onClick: () => openWorkspaceSettingsPane(CONNECTION_PANE),
                  }
                : undefined
            }
          >
            {t(unavailable ? "runtime.connection.unavailable" : "runtime.connection.reconnecting")}
          </SystemMessage>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** Composer-owned standing material shared by the floating and empty layouts.
 * An attached tray shares one pixel with the composer below; its owner restores
 * that pixel above the stack so transcript clearance still measures the complete
 * painted surface without moving any visible child. */
export function ComposerOverlayTop() {
  return (
    <Slot
      name="composer.overlay.top"
      wrapper
      className="flex w-full flex-col items-center [&:has([data-slot=composer-top-tray-surface])]:pt-px"
    />
  );
}

/**
 * The composer, resting over the tail of the transcript.
 *
 * Paints NOTHING of its own — the panel it holds is glass, and a backing behind
 * glass is just an opaque bar with a translucent sticker on it. What keeps the text
 * from colliding with the panel is the scroller's own dissolve
 * (`.msg-scroll-viewport`), which fades the last strip out; the text under the
 * panel itself stays, blurred, because that is the whole point of the material.
 *
 * Exactly the COLUMN wide, never the pane. A full-width overlay is a bottom bar
 * however it is positioned: it paints across the whole pane, which reads as
 * chrome, and it takes the scrollbar's bottom inch with it. Nothing outside the
 * column has anything to hide anyway — the transcript is centred and capped.
 */
export function FloatingComposer({
  overlayRef,
  children,
}: {
  /** Shared with ChatStream, the layout owner that reserves this overlay's height. */
  overlayRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  return (
    <div
      ref={overlayRef}
      className={cn("pointer-events-none absolute inset-x-0 bottom-0 z-2", READING_COLUMN)}
    >
      <div className={cn(READING_GUTTER, "pb-3 sm:pb-4")}>
        <div className="pointer-events-auto relative">
          <JumpToBottomButton />
          <ComposerOverlayTop />
          <RuntimeConnectionNotice />
          {children}
        </div>
      </div>
    </div>
  );
}
